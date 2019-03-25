package taskeeper

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	daySec    = 86400
	hourSec   = 3600
	minuteSec = 60
)

//GetParentDir 获取父级目录地址
func GetParentDir(p string) string {
	p = strings.Replace(p, "\\", sysDirSep, -1)
	p = strings.Replace(p, "/", sysDirSep, -1)
	parr := strings.Split(p, sysDirSep)
	return strings.Join(parr[:len(parr)-1], sysDirSep)
}

//获取随机字符串
func getChar() string {
	switch rand.Intn(3) {
	case 0:
		return string(65 + rand.Intn(90-65))
	case 1:
		return string(97 + rand.Intn(122-97))
	default:
		return strconv.Itoa(rand.Intn(9))
	}
}

//GetPidFile 获取主程序pid文件的储存路径
func GetPidFile() string {
	return pidPath
}

//GetChildPidsFile 获取主程序控制的子程序pid文件储存路径
func GetChildPidsFile() string {
	return cPidPath
}

//GetTCPAddr 获取tcp启动地址
func GetTCPAddr() string {
	return configPort
}

//ParsePidDesc 获取主进程的描述信息
func ParsePidDesc() (ProcessConfig, error) {
	p := ProcessConfig{}
	_, err := os.Stat(pidDescPath)
	if os.IsNotExist(err) {
		return p, err
	}
	data, err := ioutil.ReadFile(pidDescPath)
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p)
	if err != nil {
		return p, err
	}
	return p, nil
}

//格式化秒数
//0 d 1 h 30 m 15 s
func formatSeconds(s int64) string {
	buf := make([]string, 0, 10)

	if d := math.Floor(float64(s / daySec)); d > 0 {
		s -= int64(d) * daySec
		buf = append(buf, strconv.Itoa(int(d)))
		buf = append(buf, "d")
	}
	if h := math.Floor(float64(s / hourSec)); h > 0 {
		s -= int64(h) * hourSec
		buf = append(buf, strconv.Itoa(int(h)))
		buf = append(buf, "h")
	}
	if m := math.Floor(float64(s / minuteSec)); m > 0 {
		s -= int64(m) * minuteSec
		buf = append(buf, strconv.Itoa(int(m)))
		buf = append(buf, "m")
	}
	buf = append(buf, strconv.Itoa(int(s)))
	buf = append(buf, "s")
	return strings.Join(buf, " ")
}

//格式化日期
//2018-11-16 16:32:33
func formatDate(s int64) string {
	return time.Unix(s, 0).Format("2006-01-02 15:04:05")
}

//校验命令 补全系统路径
func checkCommand(cmd string) string {
	if !path.IsAbs(cmd) {
		var dirname, filename string
		if dirname, filename = path.Split(cmd); dirname != "" {
			return cmd
		}

		var cmdName string
		pth := os.Getenv("PATH")
		pths := strings.Split(pth, ":")
		if len(pths) > 0 {
			for _, p := range pths {
				cmdName = p + string(os.PathSeparator) + filename
				if _, err := os.Stat(cmdName); err == nil {
					cmd = cmdName
					break
				}
			}
		}
	}

	return cmd
}
