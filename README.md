# gobench

a HTTP/HTTPS load testing and benchmarking tool supporting http uploading. Originated from [gobench](https://github.com/cmpxchg16/gobench).

```bash
$ gobench -u http://127.0.0.1:5003/api/demo -n 1000
Dispatching 100 goroutines
Waiting for results...

Total Requests:                 1000 hits
Successful requests:            1000 hits
Network failed:                 0 hits
Bad requests(!2xx):             0 hits
Successful requests rate:       10754 hits/sec
Read throughput:                2.9 MiB/sec
Write throughput:               987 KiB/sec
Test time:                      92.985453ms

$ gobench
Usage: gobench [options...]

Options:
  -u URL list (comma separated), or @URL's file path (line separated)
  -m HTTP method(GET, POST, PUT, DELETE, HEAD, OPTIONS and etc)
  -c Number of connections (default 100)
  -n Number of total requests
  -t Number of concurrent goroutines (default 100)
  -d Duration of time (eg 10s, 10m, 2h45m) (default "0s")
  -p 0:Print http response; 1:with extra newline; x.log: log file
  -x proxy url, like socks5://127.0.0.1:1080, http://127.0.0.1:1080
  -post HTTP POST data
  -post.file  HTTP POST data file path
  -content.type Content-Type, eg, json, plain, or other full name
  -head.auth Authorization header
  -keepalive HTTP keep-alive (default true)
  -ok condition like 'status == 200' for json output
  -png Upload random png images by file upload
  -png.size Upload fixed img size (eg. 44kB, 17MB)
  -upload.file HTTP upload file path
  -upload.filename  Upload file name (default "file")
  -r  Number of requests per goroutine
  -read.timeout  Read timeout (in milliseconds) (default 5000)
  -write.timeout Write timeout (in milliseconds) (default 5000)
  -cpus Number of used cpu cores. (default for current machine is 12 cores)
  -think Think time, eg. 1s, 100ms, 100-200ms and etc. (unit ns, us/µs, ms, s, m, h)
  -v Print version
```

## Features

- [x] [seeweedfs](https://github.com/chrislusf/seaweedfs) bench support like `gobench -weed=http://192.168.126.5:3333 -upload.file=./weedfiles -n=10000 -think 100-500ms -p 1`

HTTP upload server benchmark in few languages/frameworks and gobench tool

## Golang upload server

## Installing

`go get github.com/bingoohuang/gobench/...`

## Usage

```bash
$ go get -u github.com/bingoohuang/gobench/cmd/go-upload-server
$ go-upload-server -sampleFileSize 1MiB
fixFile /tmp/fix1048576
$ ll -lh /tmp/fix1048576
-rw-r--r--  1 bingoobjca  wheel   1.0M 11  5 16:53 /tmp/fix1048576
$ go-upload-server  &
[1] 48324
$ curl -F 'file=@/tmp/fix1048576'  http://127.0.0.1:8811/upload
FormFile time 1.00350309s
tempFile /tmp/fub1
cost time 1.004544774s
$ kill 48324
[1]  + 48324 terminated  go-upload-server
$ go-upload-server -impl fasthttp &
[1] 48383
$ curl -F 'file=@/tmp/fix1048576'  http://127.0.0.1:8811/upload
FormFile time 576ns
tempFile /tmp/fub1
cost time 2.102639ms
$ kill 48383
[1]  + 48383 terminated  go-upload-server -impl fasthttp
$
```

1. benchmarking: `cd scripts; ./start-jmeter-workbench.sh`

## Result

Environment:

> bingoo@bingodeMacBook-Pro ~> system_profiler SPHardwareDataType
> Hardware:
>
>      Hardware Overview:
>
>        Model Name: MacBook Pro
>        Model Identifier: MacBookPro15,1
>        Processor Name: Intel Core i7
>        Processor Speed: 2.6 GHz
>        Number of Processors: 1
>        Total Number of Cores: 6
>        L2 Cache (per Core): 256 KB
>        L3 Cache: 9 MB
>        Memory: 16 GB

1. net/http upload without temp file (5 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 115     | 107    | 175      | 202      | 264      | 14  | 887 | 0.000%  | 171.53983  | 12.56           | 878361.45   |
| 总体      | 20000     | 115     | 107    | 175      | 202      | 264      | 14  | 887 | 0.000%  | 171.53983  | 12.56           | 878361.45   |

2. fasthttp upload without temp file (5 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 127     | 122    | 183      | 205      | 251      | 17  | 400 | 0.000%  | 154.58101  | 14.04           | 791524.64   |
| 总体      | 20000     | 127     | 122    | 183      | 205      | 251      | 17  | 400 | 0.000%  | 154.58101  | 14.04           | 791524.64   |

3.1. spring-boot upload with temporary store (5 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 476     | 477    | 530      | 553      | 611      | 65  | 768 | 0.000%  | 41.83085   | 4.70            | 214192.88   |
| 总体      | 20000     | 476     | 477    | 530      | 553      | 611      | 65  | 768 | 0.000%  | 41.83085   | 4.70            | 214192.88   |

3.2. pring-boot upload with temporary store (2 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 185     | 186    | 208      | 215      | 236      | 18  | 342 | 0.000%  | 107.12201  | 12.03           | 219434.30   |
| 总体      | 20000     | 185     | 186    | 208      | 215      | 236      | 18  | 342 | 0.000%  | 107.12201  | 12.03           | 219434.30   |

4.1. spring-boot upload without temporary store (5 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 151     | 132    | 279      | 369      | 456      | 20  | 753 | 0.000%  | 130.06184  | 14.61           | 665975.44   |
| 总体      | 20000     | 151     | 132    | 279      | 369      | 456      | 20  | 753 | 0.000%  | 130.06184  | 14.61           | 665975.44   |

4.2. spring-boot upload without temporary store (2 mib random context file).

| Label     | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec |
| --------- | --------- | ------- | ------ | -------- | -------- | -------- | --- | --- | ------- | ---------- | --------------- | ----------- |
| HTTP 请求 | 20000     | 61      | 36     | 133      | 183      | 311      | 7   | 506 | 0.000%  | 319.89252  | 35.93           | 655284.50   |
| 总体      | 20000     | 61      | 36     | 133      | 183      | 311      | 7   | 506 | 0.000%  | 319.89252  | 35.93           | 655284.50   |

## resources

1. [denji/awesome-http-benchmark](https://github.com/denji/awesome-http-benchmark)
