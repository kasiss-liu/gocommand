package taskeeper

import (
	"log"
	"os"
)

//Command 执行命令的配置结构
//重新封装了cmd
type Command struct {
	id          string
	pid         int
	cmd         string
	args        []string
	output      string
	isCron      bool
	cronExpress string
	process     *os.Process
}

//设置命令为cron命令
func (c *Command) SetCron(express string) *Command {
	c.isCron = true
	c.cronExpress = express
	return c
}

//验证是否是cron命令
func (c *Command) IsCron() bool {
	return c.isCron
}

//获取命令输出打印位置
func (c *Command) Output() string {
	return c.output
}

//命令启动
func (c *Command) Start() int {
	var err error
	var file *os.File

	args := append([]string{c.cmd}, c.args...)
	file, _ = os.OpenFile(c.output, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0755)
	if file == nil {
		file = os.Stdout
	}
	c.process, err = os.StartProcess(c.cmd, args, &os.ProcAttr{Files: []*os.File{nil, file, file}})
	if err == nil {
		c.pid = c.process.Pid
		return c.pid
	}

	log.Println(c.cmd + " start failed : " + err.Error())
	return 0
}
func (c *Command) ID() string {
	return c.id
}

func (c *Command) SetId(id string) {
	c.id = id
}

//Pid 获取pid
func (c *Command) Pid() int {
	return c.pid
}

//Process 获取进程结构指针
func (c *Command) Process() *os.Process {
	return c.process
}

//Kill 杀死进程
func (c *Command) Kill() error {
	if c.process != nil {
		return c.process.Kill()
	}
	process, err := os.FindProcess(c.pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

//Wait 等待进程执行完毕
//进程结束后 会释放进程资源
func (c *Command) Wait() (*os.ProcessState, error) {
	return c.process.Wait()
}

//Signal 向进程传递信号
func (c *Command) Singal(sig os.Signal) error {
	return c.process.Signal(sig)
}

//Release 释放进程资源
//释放以后 不能对进程进行任何操作
func (c *Command) Release() error {
	return c.process.Release()
}

//返回一个等待执行的cmd结构体
func NewCommand(cmd string, args []string, output string) *Command {
	return &Command{
		cmd:         cmd,
		args:        args,
		output:      output,
		isCron:      false,
		cronExpress: "",
	}
}
