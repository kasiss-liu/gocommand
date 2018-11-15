package taskeeper

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	daySec    = 86400
	hourSec   = 3600
	minuteSec = 60
)

//获取父级目录地址
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

func GetPidFile() string {
	return pidPath
}

func GetChildPidsFile() string {
	return cPidPath
}

func GetTcpAddr() string {
	return configPort
}

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

func formatDate(s int64) string {
	return time.Unix(s, 0).Format("2006-01-02 15:04:05")
}
