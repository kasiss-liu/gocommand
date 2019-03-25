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
	//StateCopy 监控服务状态的备份 用于返回客户端查询请求
	StateCopy CopyState
	//StartTime 监控服务启动时间
	StartTime int64
	//ReloadTime 监控服务重载的时间点
	ReloadTime []int64
	fmtIdent   = strings.Repeat(" ", 4)
	fmtPrefix  = ""
)

//CopyState 运行状态结构备份
type CopyState struct {
	TasksNum   int //程序总的运行数量
	RunningNum int //正在运行的命令数
	BrokenNum  int //由于崩溃或结束运行的命令数

	RunningList  map[string]*Command //正在运行的命令map
	BrokenList   map[string]*Command //运行中断的命令map
	BrokenTries  map[string]int      //命令中断后在容忍间隔时间内的重试次数
	BrokenPoints map[string]int64    //命令中断的时间点
	CronState    bool                //是否已经开启cron协程
	SecCronList  map[string]*Command //秒级cron列表
	MinCronList  map[string]*Command //分钟级cron列表
}

//获取一个主程序运行状态的copy
func copyState() CopyState {
	rs := CopyState{}
	rs.BrokenList = RunState.BrokenList
	rs.BrokenNum = RunState.BrokenNum
	rs.BrokenPoints = RunState.BrokenPoints
	rs.BrokenTries = RunState.BrokenTries
	rs.CronState = RunState.CronState
	rs.MinCronList = RunState.MinCronList
	rs.RunningList = RunState.RunningList
	rs.RunningNum = RunState.RunningNum
	rs.SecCronList = RunState.SecCronList
	rs.TasksNum = RunState.TasksNum
	return rs
}

//同步主程序的运行状态
func syncStateToCopy() {
	//每100毫秒同步一次运行状态 用于对客户端输出监控数据
	//需要协程启动
	go func() {
		for {
			StateCopy = copyState()
			time.Sleep(100 * time.Millisecond)
		}
	}()
	//每秒保存子进程的pid列表 写入文件
	go func() {
		for {
			time.Sleep(1 * time.Second)
			file, err := os.OpenFile(cPidPath, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)
			if err != nil {
				log.Println("sync child pids error : " + err.Error())
				return
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

//RunningStatus 服务状态
type RunningStatus struct {
	Pid            int      `json:"main_pid"`          //主程序pid
	StartTime      string   `json:"start_time"`        //主程序启动时间
	ReloadTime     []string `json:"reload_time_list"`  //主程序重载配置时间列表
	TotalTasks     int      `json:"task_total_num"`    //可以启动的子程序总数
	RunningTasks   []string `json:"running_task_list"` //正在运行的子程序命令集合
	TermTasks      []string `json:"term_task_list"`    //中断的子程序命令集合
	RunningSeconds string   `json:"running_seconds"`   //程序运行时间

	CronState   bool     `json:"cron_state"`       //是否已经开启cron协程
	SecCronList []string `json:"second_cron_list"` //秒级cron列表
	MinCronList []string `json:"minute_cron_list"` //分钟级cron列表
}

//获取监控服务的运行状态
func getRunningStatus() interface{} {
	runSec := time.Now().Unix() - StartTime
	runList := make([]string, 0, 5)
	termList := make([]string, 0, 5)

	for rid := range StateCopy.RunningList {
		runList = append(runList, rid)
	}
	for tid := range StateCopy.BrokenList {
		termList = append(termList, tid)
	}
	stString := formatDate(StartTime)
	reloadTimeString := make([]string, 0, 10)
	for _, tm := range ReloadTime {
		reloadTimeString = append(reloadTimeString, formatDate(tm))
	}

	cronState := StateCopy.CronState
	secCron := make([]string, 0, 5)
	for sid := range StateCopy.SecCronList {
		secCron = append(secCron, sid)
	}

	minCron := make([]string, 0, 5)
	for mid := range StateCopy.MinCronList {
		minCron = append(minCron, mid)
	}

	return RunningStatus{
		Pid:            MainPid,
		StartTime:      stString,
		ReloadTime:     reloadTimeString,
		TotalTasks:     RunState.TasksNum,
		RunningTasks:   runList,
		TermTasks:      termList,
		RunningSeconds: formatSeconds(runSec),
		CronState:      cronState,
		SecCronList:    secCron,
		MinCronList:    minCron,
	}
}

//CmdStatus 单个子程序的运行状态信息
type CmdStatus struct {
	ID         string `json:"id"`               //命令id
	Name       string `json:"name"`             //命令名称
	Pid        int    `json:"pid"`              //命令pid
	Cmd        string `json:"cmd"`              //命令的启动参数
	Output     string `json:"output"`           //命令输出的打印位置
	BkTimes    int    `json:"brokens"`          //中断次数
	LastBkTime string `json:"last_broken_time"` //上一次中断的时间
	IsCron     bool   `json:"is_cron"`          //是否是cron
}

//按照id 获取单个cmd的运行状态
func getCmd(cid string) interface{} {
	var cmdCopy Command

	var id = ""
	var ok = false
	//先查找name
	id, ok = findCmdIDByName(cid)
	//如果name查找失败 再查id
	if !ok {
		id, ok = findCmdID(cid)
	}
	//如果找到了id
	if ok {
		if cmd, ok := cmds[id]; ok {
			var bktimes int
			var lastbk int64
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
				Name:       cmdCopy.Name(),
				Output:     cmdCopy.Output(),
				BkTimes:    bktimes,
				LastBkTime: bk,
				Cmd:        cmdStr,
				IsCron:     cmdCopy.IsCron(),
			}
		}
	}
	log.Println("runnning state error getCmd : can not find cmd id `" + cid + "`")
	return nil
}

//按传入的id片段 查找完整的命令id
func findCmdID(id string) (string, bool) {
	for k := range cmds {
		if strings.HasPrefix(k, id) {
			return k, true
		}
	}
	return id, false
}

//根据传入的name片段 查找完整的命令id
func findCmdIDByName(name string) (string, bool) {
	for nm, id := range cmdNameMap {
		if strings.HasPrefix(nm, name) {
			return id, true
		}
	}
	return "", false
}

// 获取所有cmdList的运行状态
func getCmdList() interface{} {
	var list = make([]interface{}, 0, 5)
	for id := range cmds {
		cmd := getCmd(id)
		if cmd != nil {
			list = append(list, getCmd(id))
		}
	}
	return list
}

//给json增加锁进 适合阅读
func prettyJSON(jsonData interface{}, format bool) (string, error) {
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
	pid := MainPid
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

	descFile, err := os.OpenFile(pidDescPath, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		log.Println("pid desc save open file error : " + err.Error())
		return err
	}
	defer descFile.Close()
	pconf := getProcessConfig()
	data, err := json.Marshal(pconf)
	if err != nil {
		log.Println("pid desc save pid compact data error : " + err.Error())
		return err
	}
	_, err = io.WriteString(descFile, string(data))
	if err != nil {
		log.Println("pid desc save pid save file error : " + err.Error())
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

	switch runtime.GOOS {
	case "windows":
		_, err = os.FindProcess(pid)
		if err != nil {
			log.Println("pid check find process : " + err.Error())
			return nil
		}
	case "linux", "darwin":
		cmd := NewCommand("/bin/sh", []string{"-c", `ps -ef|cut -c 9-15 |grep ` + pidStr}, "/dev/null")
		cmd.Start()
		cmd.Wait()
		out := cmd.Output()
		if out == "" {
			return nil
		}
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

//删除进程描述文件
func delPidDescFile() error {
	var err error
	_, err = os.Stat(pidDescPath)
	if os.IsNotExist(err) {
		return nil
	}
	err = os.Remove(pidDescPath)
	if err != nil {
		log.Println("pid remove pid desc file error : " + err.Error())
		return err
	}
	return nil
}

//ProcessConfig 主程序配置结构
type ProcessConfig struct {
	//ConfigPath 启动时使用的配置文件
	ConfigPath string `json:"conf_path"`
	//TCPAddr Tcp启动地址
	TCPAddr string `json:"tcp_addr"`
	//PidFile Pid文件地址
	PidFile string `json:"pid_file"`
	//pidDesc Pid描述文件
	pidDesc string //`json:"pid_desc"`
	//SockFile sock文件存储路径
	SockFile string `json:"sock_file"`
	//ChdFile 子进程pid统一存储路径
	ChdFile string `json:"child_pids"`
	//LogFile 主程序日志打印位置
	LogFile string `json:"log_file"`
}

//获取主进程配置
func getProcessConfig() interface{} {
	pconf := ProcessConfig{
		ConfigPath: configRaw.Path(),
		TCPAddr:    configPort,
		PidFile:    pidPath,
		ChdFile:    cPidPath,
		LogFile:    logPath,
		pidDesc:    pidDescPath,
	}
	switch runtime.GOOS {
	case "windows":
	case "darwin", "linux":
		pconf.SockFile = sockPath
	}
	return pconf
}
