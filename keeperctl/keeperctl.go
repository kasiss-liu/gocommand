package main

import (
	"flag"
	"fmt"
	"net"

	tk "github.com/kasiss-liu/taskeeper"
)

func main() {
	flag.String("s", "", `ctl signal 'stop' , 'reload'`)
	flag.String("h", "", "service hostname : "+tk.DefaultHost)
	flag.String("p", "", "service port : "+tk.DefaultPort)
	flag.String("cat", "", "cat cmd status")
	ps, err := tk.ParsePidDesc()
	if err != nil {
		fmt.Println("load pid desc error : " + err.Error())
		return
	}

	//	wait := make(chan bool)
	addr := ps.TcpAddr
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("connect error : " + err.Error())
		return
	}

	n, err := conn.Write([]byte(`stat f server`))
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("write :", n)
	}
	var buf = make([]byte, 1000, 1000)
	n, err = conn.Read(buf)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println(string(buf[:n]))
	}

	//	reader := bufio.NewReader(conn)
	//	fmt.Println(1)
	//	data, err := reader.ReadString('\n')
	//	fmt.Println(2)
	//	if err != nil {
	//		fmt.Println(err.Error())
	//		return
	//	}
	//	fmt.Println(data)
}
