package taskeeper

import (
	"net"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	ok := SetWorkDir("keeper")
	t.Log("set workdir ", ok)
	go Start("config/config.yml", false, false)
	time.Sleep(1 * time.Second)

	msg, errcode := sendSignal("reload")
	t.Log(msg, errcode)

	msg, errcode = sendSignal("test")
	t.Log(msg, errcode)
	serr := setOutput(logPath)
	if serr != nil {
		t.Log("serr:" + serr.Error())
	}
	rs := getRunningStatus()
	t.Log("runstatus:", rs)
	cmdList := getCmdList()
	t.Log("cmd list :", cmdList)
	for id, _ := range cmds {
		msgProcess([]byte(`stat cmd ` + id))
		break
	}

	t.Log(GetPidFile())
	t.Log(GetChildPidsFile())
	t.Log(GetTcpAddr())
	t.Log(ParsePidDesc())
	t.Log(GetParentDir(cPidPath))

	time.Sleep(1 * time.Second)
	conn, err := net.Dial("tcp", configPort)
	if err != nil {
		t.Fatal(err.Error())
	}

	n, err := conn.Write([]byte(`stat f server\n`))
	if err != nil {
		t.Log(err.Error())
	} else {
		t.Log("write :", n)
	}
	conn.Close()

	msg, errcode = sendSignal("exit")
	t.Log(msg, errcode)

}

func TestGetFuncs(t *testing.T) {
	t.Log(SetWorkDir("keeper/keeper"))
	t.Log(delChildPidsFile())
	t.Log(checkPidFile())
	t.Log(delPidFile())
	t.Log(GetPidFile())
	t.Log(GetChildPidsFile())
	t.Log(GetTcpAddr())
	t.Log(ParsePidDesc())
	t.Log(GetParentDir(cPidPath))
}

func TestUnixSock(t *testing.T) {

	unixListen()
	time.Sleep(1 * time.Second)

	stopListenSerivce()
}
