package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SERVER_NETWORK = "tcp"
	SERVER_ADDRESS = "127.0.0.1:8085"
	DELIMITER      = '\t'	//数据边界单字节字符
)

//它能够一直等到所有的goroutine执行完成，并且阻塞主线程的执行，直到所有的goroutine执行完成。
var wg sync.WaitGroup

/*************
辅助函数
功能：
	日志记录
目的：
	隔离将来可能发生的日志记录方式的变化
注意：
	如果希望传递任意类型的变参，变参类型应该制定为空接口类型：args ...interface{}
 *************/
func printLog(role string, sn int, format string, args ...interface{}) {
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	fmt.Printf("%s[%d]: %s", role, sn, fmt.Sprintf(format, args...))
}

func printServerLog(format string, args ...interface{}) {
	printLog("Server", 0, format, args...)
}

func printClientLog(sn int, format string, args ...interface{}) {
	printLog("Client", sn, format, args...)
}



func strToInt32(str string) (int32, error) {
	num, err := strconv.ParseInt(str, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("\"%s\" is not integer", str)
	}
	if num > math.MaxInt32 || num < math.MinInt32 {
		return 0, fmt.Errorf("%d is not 32-bit integer", num)
	}
	return int32(num), nil
}

func cbrt(param int32) float64 {
	return math.Cbrt(float64(param))
}

/*
千万不要使用这个版本的read函数！
	使用缓冲读取的read函数
缺点：
	会读取比足够多更多的数据到缓冲区，造成提前读取
	因为每一次调用都会建立一个新的缓冲读取器，而后使用不同的读取器在同一个连接上读取数据，而没有任何的协调机制来控制读取操作
 */


//func read(conn net.Conn) (string, error) {
//	reader := bufio.NewReader(conn)
//	readBytes, err := reader.ReadBytes(DELIMITER)
//	if err != nil {
//		return "", err
//	}
//	return string(readBytes[:len(readBytes)-1]), nil
//}


/*
作用：
	从连接中读取一段以数据分界符为结尾的数据
 */
func read(conn net.Conn) (string, error) {
	//readBytes初始化为1，防止从连接值中读取多余的数据从而对后续的读取操作造成影响
	readBytes := make([]byte, 1)
	var buffer bytes.Buffer
	for {
		//通过Read方法读取网络上接收到的数据
		_, err := conn.Read(readBytes)
		if err != nil {
			return "", err
		}
		readByte := readBytes[0]
		if readByte == DELIMITER {
			break
		}
		buffer.WriteByte(readByte)
	}
	return buffer.String(), nil
}

/*
作用：
	从以数据分界符为结尾的一段数据写入连接中
 */
func write(conn net.Conn, content string) (int, error) {
	var buffer bytes.Buffer	//声明一个Buffer类型的暂存数据，存储将要发送出去的数据
	buffer.WriteString(content)		//写入将要发送出去的内容
	buffer.WriteByte(DELIMITER)		//追加数据分界符
	return conn.Write(buffer.Bytes())
}
/*************************
	服务端
功能：
	接受客户端程序的请求，
	计算请求数据的立方根，
	并把对结果的描述返回给客户端程序
功能需求：
	根据事先约定的数据边界把请求数据切分成块
	仅接受int32类型的请求数据块
	对每个符合要求的数据块，计算立方根，生成结果描述并返回给客户端程序
	鉴别闲置的通信连接（10s）并主动关闭他们
*************************/
func serverGo() {
	//声明监听器
	//根据给定协议和地址创造监听器
	var listener net.Listener
	listener, err := net.Listen(SERVER_NETWORK, SERVER_ADDRESS)
	if err != nil {
		printServerLog("Listen Error: %s", err)
		return
	}
	//defer语句，保证在serverGo函数结束执行前关闭监听器
	defer listener.Close()
	//辅助函数，为了更好的记录日志
	printServerLog("Got listener for the server. (local address: %s)", listener.Addr())

	//一旦成功获得监听器，就开始等待客户端的连接请求
	for {
		conn, err := listener.Accept() // 阻塞直至新连接到来。
		if err != nil {
			printServerLog("Accept Error: %s", err)
		}
		printServerLog("Established a connection with a client application. (remote address: %s)",
			conn.RemoteAddr())
		//go语句，启动一个新的goroutine来并发执行handleConn函数（在服务端程序来说，必须采用并发的方式处理连接）
		go handleConn(conn)
	}
}

/*****************************
	建立连接函数
输入参数：
	conn	net.Conn类型的值
作    用：
	不断的从连接中读取数据，并把响应数据通过net.Conn传递给客户端程序，所以无需返回参数
***************************** */
func handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		wg.Done()
	}()
	for {
		//作用：关闭闲置连接，设置读操作的超时时间为10s
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		//read函数作用：从连接中读取一段以数据分界符为结尾的数据
		strReq, err := read(conn)
		if err != nil {
			if err == io.EOF {
				printServerLog("The connection is closed by another side.")
			} else {
				printServerLog("Read Error: %s", err)
			}
			break
		}
		printServerLog("Received request: %s.", strReq)

		//检查数据块是否可以转化为一个int32类型的值，如果能，就立即计算它的立方根，否则返回错误信息
		intReq, err := strToInt32(strReq)
		if err != nil {
			n, err := write(conn, err.Error())
			printServerLog("Sent error message (written %d bytes): %s.", n, err)
			continue
		}
		//cbrt函数用于计算立方根
		floatResp := cbrt(intReq)
		respMsg := fmt.Sprintf("The cube root of %d is %f.", intReq, floatResp)
		n, err := write(conn, respMsg)
		if err != nil {
			printServerLog("Write Error: %s", err)
		}
		printServerLog("Sent response (written %d bytes): %s.", n, respMsg)
	}
}

/*************************************
	客户端：
功能：
	/发送给服务端程序的每块请求数据都带有约定好的数据边界
	/根据约定好的数据边界把收到的相应数据切分成块
	/在获得期望的响应数据以后，及时关闭连接节约资源
	/严格限制耗时，不应该超过5秒
 */
func clientGo(id int) {
	defer wg.Done()
	//TCP连接的同时把超时时间设定为2秒
	conn, err := net.DialTimeout(SERVER_NETWORK, SERVER_ADDRESS, 2*time.Second)
	if err != nil {
		printClientLog(id, "Dial Error: %s", err)
		return
	}
	defer conn.Close()
	//RemoteAddr返回远端地址，LocalAddr返回本地地址
	printClientLog(id, "Connected to server. (remote address: %s, local address: %s)",
		conn.RemoteAddr(), conn.LocalAddr())
	time.Sleep(200 * time.Millisecond)	//睡眠2s，为了两端程序记录的日志更加清晰

	requestNumber := 5		//把每个客户端发送的请求数据块数量定为5个
	conn.SetDeadline(time.Now().Add(5 * time.Millisecond))	//设置超时时间为0.05s

	//发送数据块代码
	for i := 0; i < requestNumber; i++ {
		req := rand.Int31()		//随机生成一个int32类型的值
		n, err := write(conn, fmt.Sprintf("%d", req))
		if err != nil {
			printClientLog(id, "Write Error: %s", err)
			continue
		}
		printClientLog(id, "Sent request (written %d bytes): %d.", n, req)
	}

	//在把所有5个请求数据块都发出去后，客户端程序开始接收响应数据块
	for j := 0; j < requestNumber; j++ {
		strResp, err := read(conn)
		if err != nil {
			if err == io.EOF {
				printClientLog(id, "The connection is closed by another side.")
			} else {
				printClientLog(id, "Read Error: %s", err)
			}
			break
		}
		printClientLog(id, "Received response: %s.", strResp)
	}
}

func main() {
	//添加等待goroutine的数量
	wg.Add(2)		//main函数只需等待下面两个程序运行完毕即可，注意，该语句必须出现在两个程序运行前
	go serverGo()
	time.Sleep(500 * time.Millisecond)
	go clientGo(1)
	wg.Wait()		//如果不在serverGo和clientGo函数开始的时候加入defer wg.Done()，则main函数将永远在这里阻塞
}
