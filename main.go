package main

import (
	"flag"
	"log"
	"os"
)

//Command 执行命令的配置结构
type Command struct {
	pid         int
	cmd         string
	args        []string
	output      string
	isCron      bool
	cronExpress string
	process     *os.Process
}

func (c *Command) SetCron(express string) *Command {
	c.isCron = true
	c.cronExpress = express
	return c
}

func (c *Command) Start() int {
	if os.Getppid() != 1 {
		args := append([]string{c.cmd}, c.args...)
		file, err := os.OpenFile(c.output, os.O_APPEND|os.O_CREATE, 0755)
		if err != nil {
			file = nil
		}

		process, err := os.StartProcess(c.cmd, args, &os.ProcAttr{Files: []*os.File{nil, file, file}})
		if err == nil {
			c.pid = process.Pid
			c.process = process
			return c.pid
		}
	}
	log.Println(c.cmd + " start failed")
	return 0
}

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

func NewCommand(cmd string, args []string, output string) *Command {
	return &Command{
		cmd:         cmd,
		args:        args,
		output:      output,
		isCron:      false,
		cronExpress: "",
	}
}

//func RebuildCommand(pid int, cmd string, args []string, output string) *Command {
//	cmd := &Command{
//		cmd:         cmd,
//		args:        args,
//		output:      output,
//		isCron:      false,
//		cronExpress: "",
//	}

//	process, err := os.FindProcess(pid)
//	if err == nil {
//		cmd.pid = pid
//		cmd.process = process
//	}
//	return cmd

//}

func main() {

	deamon := flag.Bool("d", false, "is run in deamonize")
	config := flag.String("f", "config/config.yml", "config file in Yaml Format")
	flag.Parse()

	if *deamon {
		log.Println("is deamon")
		return
	}
	loadConfig(*config)

	log.Println("ok")

}

func loadConfig(filename string) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatal(filename + " is not exist")
		}
	}
}
