package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	tk "github.com/kasiss-liu/taskeeper"
)

func main() {
	//接收输入
	s := flag.String("s", "", `ctl signal 'exit' , 'reload'`)
	h := flag.String("h", "", "service hostname : "+tk.DefaultHost)
	p := flag.String("p", "", "service port : "+tk.DefaultPort)
	cat := flag.String("cat", "", "cat cmd status")

	flag.Parse()
	//验证主机端口 可以配置远程tcp连接
	if *h != "" && *p == "" {
		fmt.Println("hostname is input, need port string")
		return
	}
	var addr string
	if *p != "" {
		addr = *h + ":" + *p
	} else {
		ps, err := tk.ParsePidDesc()
		if err != nil {
			fmt.Println("load pid desc error : " + err.Error())
			return
		}
		addr = ps.TCPAddr
	}
	//解析请求字符串
	requestString := getRequestData(*s, *cat)
	if requestString == "" {
		return
	}
	//如果解析成功 发起tcp请求
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("connect error : " + err.Error())
		return
	}
	defer conn.Close()
	n, err := conn.Write([]byte(requestString))
	fmt.Println(requestString)
	if err != nil {
		fmt.Println(err.Error())
	}
	var buf = make([]byte, 1000, 1000)
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	//打印请求结果
	data := string(buf[:n])
	dataArr := strings.Split(data, "|")
	fmt.Println(dataArr[len(dataArr)-1])

}

//解析输入 返回请求的格式化数据
func getRequestData(signal, cat string) string {
	if signal != "" {
		if _, ok := tk.SigMap[signal]; ok {
			if signal == "act" {
				if len(os.Args) < 5 {
					return ""
				}
				os.Args[3] = switchAct(os.Args[3])
				if os.Args[3] == "" {
					return ""
				}
				return tk.MsgSigCtl + " " + strings.Join(os.Args[2:], " ")
			}
			return tk.MsgSigCtl + " " + signal
		}
		fmt.Println("undefined ctl " + signal)
		return ""
	}
	if cat != "" {
		for k, arg := range tk.StatArgsMap {
			if cat == arg {
				if k > 0 {
					return tk.MsgSigStat + " f " + cat
				}
				return tk.MsgSigStat + " f " + cat + " " + getCmdID()
			}
		}
		fmt.Println("undefined cat args : " + cat)
		return ""
	}
	fmt.Println("invalid input info : -s " + signal + " -cat " + cat)
	return ""
}

//ge查询cmd的id
func getCmdID() string {
	for k, v := range os.Args {
		if v == tk.StatArgsMap[0] {
			if len(os.Args) > k {
				return os.Args[k+1]
			}
			break
		}
	}
	return ""
}

func switchAct(a string) string {

	switch a {
	case "reload":
		return strconv.Itoa(tk.ActReload)
	case "start":
		return strconv.Itoa(tk.ActStart)
	case "exit":
		return strconv.Itoa(tk.ActExit)
	case "pause":
		return strconv.Itoa(tk.ActPause)
	case "exec":
		return strconv.Itoa(tk.ActExec)
	default:
		fmt.Println("need act type!")
		return ""
	}
}
