package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	StateCopy  State   //监控服务状态的备份 用于返回客户端查询请求
	StartTime  int64   //监控服务启动时间
	ReloadTime []int64 //监控服务重载的时间点
)

func copyState() State {
	return *RunState
}

func syncStateToCopy() {
	//每100毫秒同步一次运行状态 用于对客户端输出监控数据
	//需要协程启动
	go func() {
		for {
			StateCopy = copyState()
			time.Sleep(100 * time.Microsecond)
		}
	}()
	//每秒保存子进程的pid列表 写入文件
	go func() {
		for {
			time.Sleep(1 * time.Second)
			file, err := os.OpenFile(cPidPath, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)
			if err != nil {
				log.Println("sync child pids error : " + err.Error())
				break
			}
			var pids = make([]string, 0, 5)
			for _, cmd := range StateCopy.RunningList {
				pids = append(pids, strconv.Itoa(cmd.Pid()))
			}
			file.WriteString(strings.Join(pids, "|"))
			file.Close()
		}
	}()
}

//删除保存子进程pid的文件
func delChildPidsFile() error {
	_, err := os.Stat(cPidPath)
	if os.IsNotExist(err) {
		return nil
	}
	err = os.Remove(cPidPath)
	if err != nil {
		log.Println("pid child pid file remove error : " + err.Error())
		return err
	}
	return nil
}

//服务状态
type RunningStatus struct {
	StartTime      int64    `json:start_time`
	ReloadTime     []int64  `json:reload_time_list`
	TotalTasks     int      `json:task_total_num`
	RunningTasks   []string `json:running_task_list`
	TermTasks      []string `json:term_task_list`
	RunningSeconds int64    `json:running_seconds`
}

//获取监控服务的运行状态
func getRunningStatus() string {
	runSec := time.Now().Unix() - StartTime
	runList := make([]string, 0, 5)
	termList := make([]string, 0, 5)

	for rid, _ := range StateCopy.RunningList {
		runList = append(runList, rid)
	}
	for tid, _ := range StateCopy.BrokenList {
		termList = append(termList, tid)
	}

	data, err := json.Marshal(RunningStatus{
		StartTime:      StartTime,
		ReloadTime:     ReloadTime,
		TotalTasks:     RunState.TasksNum,
		RunningTasks:   runList,
		TermTasks:      termList,
		RunningSeconds: runSec,
	})
	if err != nil {
		log.Println("running state error getRunningStatus : " + err.Error())
		return ""
	}
	return string(data)
}

type CmdStatus struct {
	Pid        int    `json:pid`
	Cmd        string `json:cmd`
	Output     string `json:output`
	BkTimes    int    `json:brokens`
	LastBkTime int64  `json:last_broken_time`
}

//按照id 获取单个cmd的运行状态
func getCmd(id string) string {
	var cmdCopy Command
	if cmd, ok := cmds[id]; ok {
		var bktimes int = 0
		var lastbk int64 = 0
		if _, ok := StateCopy.BrokenTries[id]; ok {
			bktimes = StateCopy.BrokenTries[id]
			lastbk = StateCopy.BrokenPoints[id]
		}

		cmdCopy = *cmd
		cmdStr := cmdCopy.cmd + " " + strings.Join(cmdCopy.args, " ")
		data, err := json.Marshal(CmdStatus{
			Pid:        cmdCopy.Pid(),
			Output:     cmdCopy.Output(),
			BkTimes:    bktimes,
			LastBkTime: lastbk,
			Cmd:        cmdStr,
		})
		if err != nil {
			log.Println("running state error getCmd : " + err.Error())
			return ""
		}
		return string(data)
	}
	log.Println("runnning state error getCmd : can not find cmd id `" + id + "`")
	return ""
}

// 获取所有cmdList的运行状态
func getCmdList() string {
	var list = make([]string, 0, 5)
	for id, _ := range cmds {
		list = append(list, getCmd(id))
	}
	data, err := json.Marshal(list)
	if err != nil {
		log.Println("running state error getCmdList : " + err.Error())
		return ""
	}
	return string(data)
}

//保存主进程id
func savePid() error {
	pid := os.Getpid()
	file, err := os.OpenFile(pidPath, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		log.Println("pid save open file error : " + err.Error())
		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, strconv.Itoa(pid))
	if err != nil {
		log.Println("pid save pid save file error : " + err.Error())
		return err
	}
	return nil
}

//验证主进程pid文件是否可用
func checkPidFile() error {
	var err error
	_, err = os.Stat(pidPath)
	if os.IsNotExist(err) {
		return nil
	}
	f, err := os.Open(pidPath)
	if err != nil {
		log.Println("pid check pid file error : " + err.Error())
		return err
	}
	buf := make([]byte, 10, 10)
	num, err := f.Read(buf)
	if err != nil {
		log.Println("pid check pid file error : " + err.Error())
		return err
	}

	pidStr := string(buf[:num])
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		log.Println("pid check conv pid error : " + err.Error())
		return nil
	}
	_, err = os.FindProcess(pid)
	//	process, err := os.FindProcess(pid)
	//	log.Println(process, err)
	if err != nil {
		log.Println("pid check find process : " + err.Error())
		return nil
	}
	errMsg := "pid check pid file error : process " + pidStr + " alive !"
	log.Println(errMsg)
	return errors.New(errMsg)

}

//进程结束时 删除pid文件
func delPidFile() error {
	var err error
	_, err = os.Stat(pidPath)
	if os.IsNotExist(err) {
		return nil
	}

	err = os.Remove(pidPath)
	if err != nil {
		log.Println("pid remove pid file error : " + err.Error())
		return err
	}
	return nil
}
