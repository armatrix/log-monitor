package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	client "github.com/influxData/influxdb1-client/v2"
)

//Reader 抽象input
type Reader interface {
	Read(rc chan []byte)
}
//Writer 抽象output
type Writer interface {
	Write(wc chan *Message)
}

//Message 监控数据结构
type Message struct {
	TimeLocal                    time.Time  //时间
	BytesSent                    int  //发送数据
	Path, Method, Scheme, Status string //路径、方法、协议、状态
	UpstreamTime, RequestTime    float64 
}

//SystemInfo 系统状态监控
type SystemInfo struct {
	HandleLine   int     `json:"handle_line"` // 总处理日志行数
	Tps          float64 `json:"tps"`         // 系统吞吐量
	ReadChanLen  int     `json:"read_chan_len"`
	WriteChanLen int     `json:"write_chan_len"`
	RunTime      string  `json:"run_time"` // 运行总时间
	ErrNum       int     `json:"err_num"`  // 错误数
}

const (
	//TypeHandleLine 正确处理
	TypeHandleLine = 0
	//TypeErrNum 错误处理
	TypeErrNum     = 1
)

//TypeMonitorChan 防止数据竞态
var TypeMonitorChan = make(chan int, 200)

//Monitor  监控信息
type Monitor struct {
	StartTime time.Time
	Data      SystemInfo
	TPSSli    []int
}

func (m *Monitor) start(lp *LogProcess) {
	go func() {
		for n := range TypeMonitorChan {
			switch n {
			case TypeErrNum:
				m.Data.ErrNum++
			case TypeHandleLine:
				m.Data.HandleLine++
			}
		}
	}()

	ticker := time.NewTicker(time.Second * 5)
	go func() {
		for {
			<-ticker.C
			m.TPSSli = append(m.TPSSli, m.Data.HandleLine)
			if len(m.TPSSli) > 2 {
				m.TPSSli = m.TPSSli[1:]
			}

		}
	}()

	//类似nginx的status
	http.HandleFunc("/status", func(writer http.ResponseWriter, request *http.Request) {
		m.Data.RunTime = time.Now().Sub(m.StartTime).String()
		m.Data.ReadChanLen = len(lp.rc)
		m.Data.WriteChanLen = len(lp.wc)
		if len(m.TPSSli) >= 2 {
			m.Data.Tps = float64(m.TPSSli[1]-m.TPSSli[0]) / 5
		}
		js, _ := json.MarshalIndent(m.Data, "", "\t")
		io.WriteString(writer, string(js))
	})

	http.ListenAndServe(":9193", nil)
}

//LogProcess 核心结构封装
type LogProcess struct {
	rc    chan []byte   //读取模块和解析模块的channel
	wc    chan *Message //解析模块和写入模块的channel
	read  Reader
	write Writer
}

//ReadFromFile 将不同的实现抽取出来
type ReadFromFile struct {
	path string // 为读取模块准备，读取文件的路径
}

func (r *ReadFromFile) Read(rc chan []byte) {

	f, err := os.Open(r.path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	//从文件末尾开始逐行读取文件内容
	//将文件指针移动到末尾
	f.Seek(0, 2)

	rd := bufio.NewReader(f)

	for {
		line, err := rd.ReadBytes('\n')
		// 这里的err有很多种情况，一种比如说读到末尾了，比日志数据产生的速度还要快，返回的是EOF，注意兼容这种情况
		if err == io.EOF {
			//当遇到日志文件分割时，需给rd重新赋值并重新打开文件
			time.Sleep(time.Millisecond * 500)
			continue
		} else if err != nil {
			log.Fatal(err)
		}
		TypeMonitorChan <- TypeHandleLine
		//rc <- line
		//处理读到的换行符,去掉最后的换行符
		rc <- line[:len(line)-1]
	}

}

//WriteToInfluxDB  抽象不同的output
type WriteToInfluxDB struct {
	influxDBDsn string // 为写入模块准备， influxDB Data source
}

//模拟数据
func (w *WriteToInfluxDB) Write(wc chan *Message) {
	infSli := strings.Split(w.influxDBDsn, "@")
	// Create a new HTTPClient
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     infSli[0],
		Username: infSli[1],
		Password: infSli[2],
	})
	if err != nil {
		log.Fatal(err)
	}

	for v := range wc {
		// Create a new point batch
		bp, err := client.NewBatchPoints(client.BatchPointsConfig{
			Database: infSli[3],
			//精度 秒
			Precision: infSli[4],
		})
		if err != nil {
			log.Fatal(err)
		}

		// Create a point and add to batch
		// Tags: Path, Method, Scheme, Status
		tags := map[string]string{"path": v.Path, "method": v.Method, "scheme": v.Scheme, "status": v.Status}
		// Fields: UpstreamTime, RequestTime, BytesSent
		fields := map[string]interface{}{
			"upstream-time": v.UpstreamTime,
			"request-time":  v.RequestTime,
			"bytes-sent":    v.BytesSent,
		}

		pt, err := client.NewPoint("nginx_log", tags, fields, v.TimeLocal)
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)

		// Write the batch
		if err := c.Write(bp); err != nil {
			log.Fatal(err)
		}

		log.Println("write to influxdb success!")
	}
}

//Process 解析模块
func (l *LogProcess) Process() {

	// 172.0.0.12 - - [2020-08-03T21:25:48+08:00] https "GET /foo HTTP/1.0" 200 1905 "-" "KeepAliveClient" "-" - 1.470
	r := regexp.MustCompile(`([\d\.]+)\s+([^ \[]+)\s+([^ \[]+)\s+\[([^\]]+)\]\s+([a-z]+)\s+\"([^"]+)\"\s+(\d{3})\s+(\d+)\s+\"([^"]+)\"\s+\"(.*?)\"\s+\"([\d\.-]+)\"\s+([\d\.-]+)\s+([\d\.-]+)`)

	for v := range l.rc {
		ret := r.FindStringSubmatch(string(v))
		//上面有13个括号，正常应该是匹配到13组数据，如果没有匹配到的错误处理  注意这里会得到14的长度，日志的本身是第零项
		if len(ret) != 14 {
			TypeMonitorChan <- TypeErrNum
			log.Println("FindStringSubMatch failed:", string(v))
			continue
		}

		message := &Message{}
		loc, _ := time.LoadLocation("Asia/Shanghai")
		t, err := time.ParseInLocation(time.RFC3339, ret[4], loc)
		if err != nil {
			TypeMonitorChan <- TypeErrNum
			log.Println("ParseInLocation failed: ", err.Error(), ret[4])
		}
		message.TimeLocal = t

		byteSent, _ := strconv.Atoi(ret[8])
		message.BytesSent = byteSent

		//"GET /foo?query=t HTTP/1.0"
		reqSli := strings.Split(ret[6], " ")
		if len(reqSli) != 3 {
			TypeMonitorChan <- TypeErrNum
			log.Println("strings.split failed: ", ret[6])
			continue
		}
		message.Method = reqSli[0]

		u, err := url.Parse(reqSli[1])
		if err != nil {
			TypeMonitorChan <- TypeErrNum
			log.Println("url parse failed: ", err)
			continue
		}
		message.Path = u.Path

		message.Scheme = ret[5]
		message.Status = ret[7]

		upstreamTime, _ := strconv.ParseFloat(ret[12], 64)
		requestTime, _ := strconv.ParseFloat(ret[13], 64)

		message.UpstreamTime = upstreamTime
		message.RequestTime = requestTime

		l.wc <- message
	}
}

func main() {
	//参数的传入不需要重新编译文件   注意所有的编写都应该考虑到
	var path, influxDsn string
	flag.StringVar(&path, "path", "../access.log", "log path")
	flag.StringVar(&influxDsn, "influxDsn", "http://127.0.0.1:8086@someuser@somepassword@mydb@s", "Dsn")
	flag.Parse()

	r := &ReadFromFile{
		path: path,
	}

	w := &WriteToInfluxDB{
		influxDBDsn: influxDsn,
	}

	lp := &LogProcess{
		//设置缓冲消费队列
		rc:    make(chan []byte,200),
		wc:    make(chan *Message,200),
		read:  r,
		write: w,
	}

	// 处理模块相对read模块较慢，存在正则等问题，写入模块则更慢，通过多个goroutine来协调并发，同设置缓冲队列的参数一样，最好通过配置传入
	go lp.read.Read(lp.rc)
	for i := 0; i < 2; i++ {
		go lp.Process()
	}
	for i := 0; i < 6; i++ {
		go lp.write.Write(lp.wc)
	}

	m := &Monitor{
		StartTime: time.Now(),
		Data:      SystemInfo{},
	}
	m.start(lp)
}
