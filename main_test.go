package main

import (
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	go func() {
		time.Sleep(3 * time.Second)
		msg, err := sendSignal("reload")
		t.Log(msg, err)
	}()
	go func() {
		time.Sleep(6 * time.Second)
		msg, err := sendSignal("exit")
		t.Log(msg, err)
	}()
	go func() {
		msg, err := sendSignal("test")
		t.Log(msg, err)
		time.Sleep(1 * time.Second)
		rs := getRunningStatus()
		t.Log("runstatus:", rs)
		cmdList := getCmdList()
		t.Log("cmd list :", cmdList)
		for id, _ := range cmds {
			msgProcess([]byte(`stat cmd ` + id))
			break
		}
	}()
	main()
}
