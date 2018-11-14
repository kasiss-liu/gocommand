package taskeeper

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	StateCopy  State   //监控服务状态的备份 用于返回客户端查询请求
	StartTime  int64   //监控服务启动时间
	ReloadTime []int64 //监控服务重载的时间点
	fmtIdent   = strings.Repeat(" ", 4)
	fmtPrefix  = ""
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
	StartTime      int64    `json:"start_time"`
	ReloadTime     []int64  `json:"reload_time_list"`
	TotalTasks     int      `json:"task_total_num"`
	RunningTasks   []string `json:"running_task_list"`
	TermTasks      []string `json:"term_task_list"`
	RunningSeconds int64    `json:"running_seconds"`
}

//获取监控服务的运行状态
func getRunningStatus() interface{} {
	runSec := time.Now().Unix() - StartTime
	runList := make([]string, 0, 5)
	termList := make([]string, 0, 5)

	for rid, _ := range StateCopy.RunningList {
		runList = append(runList, rid)
	}
	for tid, _ := range StateCopy.BrokenList {
		termList = append(termList, tid)
	}

	return RunningStatus{
		StartTime:      StartTime,
		ReloadTime:     ReloadTime,
		TotalTasks:     RunState.TasksNum,
		RunningTasks:   runList,
		TermTasks:      termList,
		RunningSeconds: runSec,
	}
}

type CmdStatus struct {
	ID         string `json:"id"`
	Pid        int    `json:"pid"`
	Cmd        string `json:"cmd"`
	Output     string `json:"output"`
	BkTimes    int    `json:"brokens"`
	LastBkTime string `json:"last_broken_time"`
}

//按照id 获取单个cmd的运行状态
func getCmd(id string) interface{} {
	var cmdCopy Command

	if id, ok := findCmdId(id); ok {
		if cmd, ok := cmds[id]; ok {
			var bktimes int = 0
			var lastbk int64 = 0
			if _, ok := StateCopy.BrokenTries[id]; ok {
				bktimes = StateCopy.BrokenTries[id]
				lastbk = StateCopy.BrokenPoints[id]
			}

			cmdCopy = *cmd
			var bk = "null"
			if lastbk > 0 {
				bk = time.Unix(lastbk, 0).Format("2006-01-02 15:04:05")
			}

			cmdStr := cmdCopy.cmd + " " + strings.Join(cmdCopy.args, " ")
			return CmdStatus{
				ID:         cmdCopy.ID(),
				Pid:        cmdCopy.Pid(),
				Output:     cmdCopy.Output(),
				BkTimes:    bktimes,
				LastBkTime: bk,
				Cmd:        cmdStr,
			}
		}
	}
	log.Println("runnning state error getCmd : can not find cmd id `" + id + "`")
	return nil
}

//按传入的id片段 查找完整的命令id
func findCmdId(id string) (string, bool) {
	for k, _ := range cmds {
		if strings.HasPrefix(k, id) {
			return k, true
		}
	}
	return id, false
}

// 获取所有cmdList的运行状态
func getCmdList() interface{} {
	var list = make([]interface{}, 0, 5)
	for id, _ := range cmds {
		cmd := getCmd(id)
		if cmd != nil {
			list = append(list, getCmd(id))
		}
	}
	return list
}

//给json增加锁进 适合阅读
func prettyJson(jsonData interface{}, format bool) (string, error) {
	var byteData []byte
	var err error
	if format {
		byteData, err = json.MarshalIndent(jsonData, fmtPrefix, fmtIdent)
	} else {
		byteData, err = json.Marshal(jsonData)
	}
	if err == nil {
		return string(byteData), nil
	}
	return "", err
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

type ProcessConfig struct {
	ConfigPath string `json:"conf_path"`
	TcpAddr    string `json:"tcp_addr"`
	PidFile    string `json:"pid_file"`
	SockFile   string `json:"sock_file"`
	ChdFile    string `json:"child_pids"`
	LogFile    string `json:"log_file"`
}

//获取主进程配置
func getProcessConfig() interface{} {
	pconf := ProcessConfig{
		ConfigPath: configRaw.Path(),
		TcpAddr:    configPort,
		PidFile:    pidPath,
		ChdFile:    cPidPath,
		LogFile:    logPath,
	}
	switch runtime.GOOS {
	case "windows":
	case "darwin", "linux":
		pconf.SockFile = sockPath
	}
	return pconf
}
