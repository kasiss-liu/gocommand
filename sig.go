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
	sigReload = 1 //重新读取配置 并重新启动所有协程
	sigExit   = 0 //管理进程退出
	sigStart  = 2 //启动协程
	sigCtlCmd = 3 //单独控制启动某个命令
)

const (
	//ActReload 重新读取配置 并重新启动所有协程
	ActReload = sigReload
	//ActExit 管理进程退出
	ActExit = sigExit
	//ActStart 启动协程
	ActStart = sigStart
)

//定义一些常量 错误编号
const (
	ErrResCodeNo  = iota //无错误 0
	ErrResWrgMsg         //消息结构不正确 1
	ErrResUdfCtl         //未定义的操作命令 2
	ErrResMissCmd        //缺少命令ID 3
	ErrResStatNil        //未获取到合法的参数 4
	ErrResWrgSig         //未定义的信号 5
	ErrResCtlSig         //缺少需要重启的命令id 6
)

//ErrMsgMap 错误编号对应的消息数组
var ErrMsgMap = []string{
	"success",
	"wrong message",
	"undefined ctl type",
	"miss cmd id",
	"found nil args",
	"undefined signal",
	"undefined restart cmd",
}

//客户端操作命令常量
const (
	MsgSigCtl  = "ctl"  //控制
	MsgSigStat = "stat" //查询
)

var (
	//SigMap 信号map
	SigMap map[string]int
	//StatArgsMap 信号参数map
	StatArgsMap []string
	//serviceDonw 结束服务通道
	serviceDonw chan bool
	//unixServer unix下的服务 .sock启动 暂未启用
	unixServer *net.UnixListener
	//tcpServer 所有环境下启动tcp服务
	tcpServer *net.TCPListener
	//signalChan 信号通道
	signalChan chan int
	//sysSigChan 监听系统命令 ctl+c kill 等
	sysSigChan chan os.Signal
	//msgProcessLock 消息处理锁
	msgProcessLock sync.Mutex
	//signalCmdCtlChan 命令单独控制通道
	signalCmdCtlChan chan cmdCtlAction
)

//命令控制信息结构体
type cmdCtlAction struct {
	sig   int
	cmdid string
}

func init() {
	signalChan = make(chan int)
	sysSigChan = make(chan os.Signal)
	serviceDonw = make(chan bool)
	signalCmdCtlChan = make(chan cmdCtlAction, 10)
	SigMap = map[string]int{
		"reload": sigReload,
		"exit":   sigExit,
		"act":    sigCtlCmd,
	}
	StatArgsMap = []string{
		"cmd",
		"cmdlist",
		"server",
		"config",
	}
}

//启动监听服务
//接收客户端的消息
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
		c.Write(bytes)
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
	case MsgSigCtl:
		msg, errcode := sendSignal(data[argStart:]...)
		return msg, errcode, format
	case MsgSigStat:
		msg, errcode := sendStat(data[argStart:]...)
		return msg, errcode, format
	}
	return ErrMsgMap[ErrResUdfCtl] + " : {" + data[0] + "}", ErrResUdfCtl, false
}

//状态查询方法
func sendStat(s ...string) (interface{}, int) {
	var msg interface{}
	switch s[0] {
	case StatArgsMap[0]:
		if len(s) < 2 {
			msg = ErrMsgMap[ErrResMissCmd]
			return msg, ErrResMissCmd
		}
		msg = getCmd(s[1])
	case StatArgsMap[1]:
		msg = getCmdList()
	case StatArgsMap[2]:
		msg = getRunningStatus()
	case StatArgsMap[3]:
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
	msg, _ = prettyJSON(msgData, format)
	return []byte(strconv.Itoa(errcode) + "|format:" + fmtStr + msg)
}

//控制发送信号方法
func sendSignal(ss ...string) (msg string, errcode int) {
	s := ss[0]
	//向通道内发送信号
	if sig, ok := SigMap[s]; ok {
		switch sig {
		case sigCtlCmd:
			if len(ss) < 3 {
				log.Println(ss)
				msg = ErrMsgMap[ErrResCtlSig] + " : {" + s + "}"
				errcode = ErrResCtlSig
				log.Println(msg)
			} else {
				msg = "ok"
				errcode = 0
				ctlAction, err := strconv.Atoi(ss[1])
				if err != nil {
					msg = ErrMsgMap[ErrResCtlSig] + " : {" + s + "}"
					log.Println(msg + " error :" + err.Error())
					errcode = ErrResCtlSig
				} else {
					signalCmdCtlChan <- cmdCtlAction{ctlAction, ss[2]}
					signalChan <- sig
				}
			}
		default:
			errcode = 0
			msg = "ok"
			signalChan <- sig
		}

	} else {
		msg = ErrMsgMap[ErrResWrgSig] + " : {" + s + "}"
		errcode = ErrResWrgSig

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
