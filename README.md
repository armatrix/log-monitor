# monitor

监控逻辑展示小组件

### 背景

假定我们有若干台机器，上面跑了一些程序，我们想对资源使用情况做一个大致的了解。或者是我们对某一服务的日志做一些分析，如对我们提供服务接口被调用的一些状态数据的显示。在系统的监控上，《SRE: Google运维解密》一书中指出，监控系统需要能够有效的支持白盒监控和黑盒监控。通过白盒能够了解其内部的实际运行状态，通过对监控指标的观察能够预判可能出现的问题，从而对潜在的不确定因素进行优化。而黑盒监控，常见的如HTTP探针，TCP探针等，可以在系统或者服务在发生故障时能够快速通知相关的人员进行处理。通过建立完善的监控体系，从而达到以下目的：

- 长期趋势分析：通过对监控样本数据的持续收集和统计，对监控指标进行长期趋势分析。例如，通过对磁盘空间增长率的判断，我们可以提前预测在未来什么时间节点上需要对资源进行扩容。
- 对照分析：两个版本的系统运行资源使用情况的差异如何？在不同容量情况下系统的并发和负载变化如何？通过监控能够方便的对系统进行跟踪和比较。
- 告警：当系统出现或者即将出现故障时，监控系统需要迅速反应并通知管理员，从而能够对问题进行快速的处理或者提前预防问题的发生，避免出现对业务的影响。
- 故障分析与定位：当问题发生后，需要对问题进行调查和处理。通过对不同监控监控以及历史数据的分析，能够找到并解决根源问题。
- 数据可视化：通过可视化仪表盘能够直接获取系统的运行状态、资源使用情况、以及服务运行状态等直观的信息。

一些诸如Nagios组件所能提供的功能更像是围绕一些边缘性的问题，主要针对系统服务和资源的状态以及应用程序的可用性，此类往往会存在如下问题：

- 与业务脱离的监控：监控系统获取到的监控指标与业务本身也是一种分离的关系。好比客户可能关注的是服务的可用性、服务的SLA等级，而监控系统却只能根据系统负载去产生告警；

- 运维管理难度大：Nagios这一类监控系统本身运维管理难度就比较大，需要有专业的人员进行安装，配置和管理，而且过程并不简单；

- 可扩展性低： 监控系统自身难以扩展，以适应监控规模的变化；

- 问题定位难度大：当问题产生之后（比如主机负载异常增加）对于用户而言，他们看到的依然是一个黑盒，他们无法了解主机上服务真正的运行情况，因此当故障发生后，这些告警信息并不能有效的支持用户对于故障根源问题的分析和定位。

此类问题的解决方案可以参照Prometheus，作为新一代的云原生监控系统，其与容器平台、云平台融合表现优异。

我们不妨从另一个角度去开始我们在这块的工作。

使用nginx做一些web服务的代理是比较常见的一种情况，假定我们想看到我们暴露的web服务的访问情况。nginx默认会产生`acess.log` 和 `error.log`, 如果我们想从类似这种日志源中看到一些直观的数据，对源日志的提取，简单的处理，存储，在接入一些图形展示的控件。这里我们通过docker快速搭建开发环境，先模拟日志文件，再接入nginx的日志，使用influxdb和grafana来完成上述操作。

很明显可以优化的地方是：我们将nginx的日志持久化存储了，又将该文件导入到influxdb中，在性能上我们更应该是将日志直接写入influxdb中。可以考虑下这里该如何做，是否要这样做，其代价和收益又是什么？

日志源从nginx换成其他的也类似。可以用来做流量监控，接口api是不是稳定，诸如此类的服务。

在手动实现基础的逻辑之后，我们通过对Prometheus的说明来完成整个流程。

监控系统引出的另外一个问题是，我们想要测量一杯水，如何去抽象测量逻辑让温度计不会影响到真实的水温。

### 涉及技术及工具

go、docker、influxdb、grafana、nginx、prometheus

### 文档说明

第一部分介绍go的一些基础概念

第二部分简单介绍influxdb、grafana

第三部分通过模拟日志源来实现我们的监控系统

第四部分简单介绍nginx，将日志源修改

第五部分Prometheus

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
docker exec -it myinfluxdb bash
influx -precision rfc3339
# 查看当前数据库
show databases;
# 新建名为mydb的数据库
create database mydb;
# 可选 设置用户和密码 注意这里密码要用单引号
create user "someuser" with password 'somepassword' with all privileges
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

### 代码实现

这里我们将功能需求列为：获取某个协议下的某个请求在某个请求方法的QPS、响应时间、流量。

需要的数据字段为：

```ini
tags: path, method, scheme, status
fields: upstream-time, request-time, bytes-sent
time: loaltime
```

模块主要分为三部分：读取、解析、写入，模块之间通过channel进行连接

#### 读取模块

1. 打开文件
2. 从文件末尾开始逐行读取最新内容
3. 写入reader channel

#### 解析模块

1. 从reader channel中读取每行日志数据
2. 通过正则提取所需监控数据（path、status、method etc.）
3. 写入writer channel

#### 写入模块

1. 初始化influxdb client
2. 从writer channel中读取监控数据
3. 构造数据并写入influxDB



