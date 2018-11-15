package taskeeper

import (
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	sigBroken = -1 //所有携程已经退出(结束或者崩溃)
	sigReload = 1  //重新读取配置 并重新启动所有协程
	sigExit   = 0  //管理进程退出
	sigStart  = 2  //启动协程
)

const (
	ErrResCodeNo = iota
	ErrResWrgMsg
	ErrResUdfCtl
	ErrResMissCmd
	ErrResStatNil
	ErrResWrgSig
)

var ErrMsgMap = []string{
	"success",
	"wrong message",
	"undefined ctl type",
	"miss cmd id",
	"found nil args",
	"undefined signal",
}

const (
	msgSigCtl  = "ctl"
	msgSigStat = "stat"
)

var (
	sigMap         map[string]int
	statArgsMap    []string
	serviceDonw    chan bool
	unixServer     *net.UnixListener
	tcpServer      *net.TCPListener
	signalChan     chan int
	sysSigChan     chan os.Signal
	msgProcessLock sync.Mutex
)

func init() {
	signalChan = make(chan int)
	sysSigChan = make(chan os.Signal)
	serviceDonw = make(chan bool)
	sigMap = map[string]int{
		"break":  sigBroken,
		"reload": sigReload,
		"exit":   sigExit,
	}
	statArgsMap = []string{
		"cmd",
		"cmdlist",
		"server",
		"config",
	}
}

func startListenService() {
	//windows下暂时不做处理
	switch runtime.GOOS {
	case "windows":
		tcpListen()
	//macos和linux 启动unix通信
	case "darwin", "linux":
		tcpListen()
		//		unixListen()
	}
}

//启动tcp通信
func tcpListen() {
	log.Println("tcp listen service starting ...")
	var err error
	tcpAddr, err := net.ResolveTCPAddr("tcp", configPort)
	if err != nil {
		log.Fatalln("tcp listen start faild : " + err.Error())
	}
	tcpServer, err = net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Fatalln("tcp listen start faild : " + err.Error())
	}

	go func() {
		for {
			c, err := tcpServer.AcceptTCP()
			if err != nil {
				if _, ok := err.(*net.OpError); ok {
					break
				}
				log.Printf("unix listen accept error : %s\n", err.Error())
				continue
			}

			go listenHandle(c)
		}
	}()
	log.Println("tcp listen service started at " + configPort)
}

//处理消息
func listenHandle(c net.Conn) {
	for {
		var buf = make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Println("tcp client error:" + err.Error())
			}
			c.Close()
			break
		}
		msg, errcode, format := msgProcess(buf[:n])
		bytes := getResponseBytes(errcode, msg, format)
		c.Write(append(bytes, '\n'))
	}

}

//启动unix通信
func unixListen() {
	log.Println("unix listen service starting ...")
	var err error
	unixServer, err = net.ListenUnix("unix", &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		log.Fatalln("unix listen start faild : " + err.Error())
	}
	unixServer.SetUnlinkOnClose(true)

	go func() {
		for {
			c, err := unixServer.AcceptUnix()
			if err != nil {
				if _, ok := err.(*net.OpError); ok {
					break
				}
				log.Printf("unix listen accept error : %s\n", err.Error())
				continue
			}

			go listenHandle(c)
		}
	}()
	log.Println("unix listen service started")
}

//处理客户端消息内容
//消息文本格式
//`[命令类型] [命令] [参数]`
//`ctl reload|exit `
//`stat cmd id`
//`stat cmdlist`
//`stat server`
func msgProcess(msg []byte) (interface{}, int, bool) {

	msgProcessLock.Lock()
	defer msgProcessLock.Unlock()
	format := false
	argStart := 1

	msgStr := strings.TrimSpace(string(msg))

	//先暴力的去除空格 以后再优化
	datas := strings.Split(msgStr, " ")
	var data []string
	for _, v := range datas {
		if len([]byte(v)) > 0 {
			data = append(data, v)
		}
	}

	if len(data) < 2 {
		return ErrMsgMap[ErrResWrgMsg] + " : {" + msgStr + "}", ErrResWrgMsg, false
	}
	if data[1] == "f" {
		format = true
		argStart++
	}
	if format && len(data) < 3 {
		return ErrMsgMap[ErrResWrgMsg] + " : {" + msgStr + "}", ErrResWrgMsg, false
	}
	//匹配命令类型
	switch data[0] {
	case msgSigCtl:
		msg, errcode := sendSignal(data[argStart])
		return msg, errcode, format
	case msgSigStat:
		msg, errcode := sendStat(data[argStart:]...)
		return msg, errcode, format
	}
	return ErrMsgMap[ErrResUdfCtl] + " : {" + data[0] + "}", ErrResUdfCtl, false
}

//状态查询方法
func sendStat(s ...string) (interface{}, int) {
	var msg interface{}
	switch s[0] {
	case statArgsMap[0]:
		if len(s) < 2 {
			msg = ErrMsgMap[ErrResMissCmd]
			return msg, ErrResMissCmd
		}
		msg = getCmd(s[1])
	case statArgsMap[1]:
		msg = getCmdList()
	case statArgsMap[2]:
		msg = getRunningStatus()
	case statArgsMap[3]:
		msg = getProcessConfig()
	}

	if msg != nil {
		return msg, ErrResCodeNo
	}
	msg = ErrMsgMap[ErrResStatNil] + " : " + strings.Join(s, " ")
	return msg, ErrResStatNil
}

//格式化响应数据
func getResponseBytes(errcode int, msgData interface{}, format bool) []byte {
	var msg string
	fmtStr := "compact|"
	if format && errcode == 0 {
		fmtStr = "pretty|\n"
	}
	msg, _ = prettyJson(msgData, format)
	return []byte(strconv.Itoa(errcode) + "|format:" + fmtStr + msg)
}

//控制发送信号方法
func sendSignal(s string) (msg string, errcode int) {

	//向通道内发送信号
	if sig, ok := sigMap[s]; ok {
		errcode = 0
		msg = "ok"
		signalChan <- sig

	} else {
		msg = ErrMsgMap[ErrResWrgSig] + " : {" + s + "}"
		errcode = ErrResWrgSig
		log.Println(msg)
	}
	return
}

//关闭监听服务
func stopListenSerivce() {
	switch runtime.GOOS {
	case "windows":
	case "darwin", "linux":
		if unixServer != nil {
			unixServer.Close()
			log.Println("unix listen service stopped")
		}
	}
}

//启动一个协程 监听系统信号
//如果是ctrl+c 或普通 kill 信号 则发送退出指令 有序退出程序
func listenSystemSig() {
	go func() {
		sig := <-sysSigChan
		log.Println("system signal :" + sig.String())
		signalChan <- sigExit
	}()
}
