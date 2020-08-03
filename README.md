# log-monitor
simple log monitor 

### 项目背景

我们通常会用nginx做一些web服务的代理，在ngixn的日志中，通常将其分为正确的访问日志和错误的日志。我们更希望的是将这些日志可视化。这里我们通过docker快速搭建开发环境，使用influxdb和grafana将数据进行存储、展示。这里很明显可以优化的地方是：我们将nginx的日志持久化存储了，又将该文件导入到influxdb中，在性能上我们更应该是将日志直接写入influxdb中。可以考虑下这里该如何做，是否要这样做，其代价和收益又是什么？
同样的，将日志源从nginx换成其他的也类似。

### 涉及技术及工具

go、docker、influxdb、grafana、nginx

### 文档说明

第一部分介绍go的一些基础概念

第二部分简单介绍influxdb、grafana

第三部分简单介绍nginx

第四部分通过模拟日志源来实现我们的监控系统

第五部分则是简单的将nginx的日志接入即可

### 常见并发模型和go并发实现

- 进程&线程(Apache) 	引入C10K的问题
- 异步非阻塞(Nginx，Libevent，NodeJS)          引入更高的复杂度
- 协程(Golang，Erlang，Lua)

#### golang并发实现

- 程序并发执行（goroutine）
- 多个goroutine之间通过channel进行数据同步和通信
- 多个channel通过select选择数据的读取或写入

#### goroutines（程序并发执行）

```go
foo()	// 执行函数foo，程序等待函数foo返回

go foo()  // 执行函数foo
bar()     // 不用等待foo返回
```

这里引出的问题是：在多个goroutine之间数据要如何传输

#### channel（多个goroutine之间的数据通信与异步操作）

```go
c := make(chan string)   // 创建一个channel
go func(){
  time.sleep(1 * time.Second)
  c <- "message from closure"  //发送数据到channel中
}()
msg := <-c	  // 阻塞直到接收到数据
```

#### select多路复用

```go
select {
case v := <- c1 :
  fmt.Printf("channel 1 sends: %v.", v)
case v := <- c2 :
  fmt.Printf("channel 2 sends: %v.", v)
default :
  fmt.println("neither channel was ready")
}
```

#### 并发和并行

并发：单个CPU在逻辑上的同时执行

并行：多核CPU上，在物理上同时执行

在go实现中可以参考Rob Pike的 [Concurrency Is Not Parallelism](https://youtu.be/cN_DpYBzKso) 

#### golang中的“OOP” （Object Oriented Programming）

封装、继承和多态是面向对象程序设计的三大基石，在go中，前两者通过struct类型进行实现，在新的提案中已经有关于多态的实现，这里我们通过对接口的使用实现部分基础的功能

##### 封装

```go
type Foo struct{
  baz string
}

func (f *Foo) echo(){
  fmt.Println(f.baz)
}

func main(){
  f := Foo{baz: "hello, struct"}
  f.echo()
}
```

##### 继承

```go
type Foo struct{
  baz string
}

type Bar struct{
  Foo
}

func (f *Foo) echo(){
  fmt.Println(f.baz)
}

func main(){
  b := Bar{Foo{baz: "hello, struct"}}
  b.echo()
}
```

##### 多态

```go
type Foo interface{
  qux()
}

type Bar struct{}
type Baz struct{}

func (b Bar) qux(){}
func (b Baz) qux(){}

func main(){
  var f Foo
  f = Bar{}
  f = Baz{}
  fmt.Println(f)
}
```

### InfluxDB

influxDB是一个开源的时序型数据库time series database (TSDB)，使用go语言编写，被广泛应用于存储系统的监控数据的存储、IoT行业的实时数据等场景。有以下特性

- 部署简单，无外部依赖
- 内置http支持，使用http读写
- 类sql的灵活查询（max，min，sum等）

#### 关键概念

我们将数据对象描述为，在时间轴上，数据在不同维度是如何展示的。

database：数据库

measurement：类似其他数据库中的表

points：类似表里面的一行数据，其中tags：各种有索引的属性，fields：各种记录的值，time：数据记录的时间戳，也是自动生成的主索引

数据格式： `measurement-name tag-set field-set timestamp`

#### 快速开始

```shell
# 新建influxdb容器
docker run --name myinfluxdb -p 8086:8086 \
      -v $HOME/docker-volume/influxDB:/var/lib/influxdb \
      -d influxdb
# 在容器内部执行 influx 并指定时间格式进入InfluxDB shell
influx -precision rfc3339
# 查看当前数据库
show databases;
# 新建名为mydb的数据库
create database mydb;
# 使用该数据库 
use mydb;

# 在任一可进行通讯的终端使用curl来进行数据的添加和查询
# 这里我们可以假设一种场景
# 我们有若干台服务器，分布在不同的区域，cpu和gpu型号不同,我们想查看这些不同区域的cpu和gpu使用率
# 在新建的mydb下新增measurement为server，tags分别为location、cpu、gpu，fields分别为两者的使用率cpu-usage、gpu-usage，time使用默认的本地时间即可
curl -i -XPOST 'http://localhost:8086/write?db=mydb' --data-binary 'server,location=asia,cpu=amd3800,gpu=1080Ti  cpu-usage=0.2,gpu-usage=0.3'
# 查询
curl -G 'http://localhost:8086/query?pretty=true' --data-urlencode "db=mydb" --data-urlencode "q=select \"cpu-usage\" from \"server\" where \"location\"='asia'"

# 在influxdb中,对数据的修改通过相同的索引进行覆盖，删除则采用过期策略,在influx shell查询
SHOW RETENTION POLICIES ON mydb;

# 同样的， Retention Policies在创建之初指定、对已有的修改、删除分别通过如下命令实现
# 给mydb数据库的server创建一个有30天时长的副本
CREATE RETENTION POLICY "server" ON "mydb" DURATION 30d REPLICATION 1 DEFAULT
# 删除保存策略
DROP RETENTION POLICY "server" ON "mydb"
```

更多样例请参照官方文档

