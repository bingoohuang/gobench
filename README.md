# http-upload-benchmark
HTTP upload server benchmark in few languages/frameworks and gobench tool

# Golang upload server

## Installing

`go get github.com/bingoohuang/http-upload-benchmark/...`

## Usage

```bash
$ go get -u github.com/bingoohuang/http-upload-benchmark/cmd/go-upload-server
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

>  bingoo@bingodeMacBook-Pro ~> system_profiler SPHardwareDataType
>  Hardware:
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

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 115     | 107    | 175      | 202      | 264      | 14  | 887 | 0.000%  | 171.53983  | 12.56           | 878361.45   | 
| 总体     | 20000     | 115     | 107    | 175      | 202      | 264      | 14  | 887 | 0.000%  | 171.53983  | 12.56           | 878361.45   | 

2. fasthttp upload without temp file (5 mib random context file).

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 127     | 122    | 183      | 205      | 251      | 17  | 400 | 0.000%  | 154.58101  | 14.04           | 791524.64   | 
| 总体     | 20000     | 127     | 122    | 183      | 205      | 251      | 17  | 400 | 0.000%  | 154.58101  | 14.04           | 791524.64   | 


3.1. spring-boot upload with temporary store (5 mib random context file).

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 476     | 477    | 530      | 553      | 611      | 65  | 768 | 0.000%  | 41.83085   | 4.70            | 214192.88   | 
| 总体     | 20000     | 476     | 477    | 530      | 553      | 611      | 65  | 768 | 0.000%  | 41.83085   | 4.70            | 214192.88   | 


3.2. pring-boot upload with temporary store (2 mib random context file).

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 185     | 186    | 208      | 215      | 236      | 18  | 342 | 0.000%  | 107.12201  | 12.03           | 219434.30   | 
| 总体     | 20000     | 185     | 186    | 208      | 215      | 236      | 18  | 342 | 0.000%  | 107.12201  | 12.03           | 219434.30   | 


4.1. spring-boot upload without temporary store (5 mib random context file).

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 151     | 132    | 279      | 369      | 456      | 20  | 753 | 0.000%  | 130.06184  | 14.61           | 665975.44   | 
| 总体     | 20000     | 151     | 132    | 279      | 369      | 456      | 20  | 753 | 0.000%  | 130.06184  | 14.61           | 665975.44   | 


4.2. spring-boot upload without temporary store (2 mib random context file).

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 20000     | 61      | 36     | 133      | 183      | 311      | 7   | 506 | 0.000%  | 319.89252  | 35.93           | 655284.50   | 
| 总体     | 20000     | 61      | 36     | 133      | 183      | 311      | 7   | 506 | 0.000%  | 319.89252  | 35.93           | 655284.50   | 


# gobench

It is a HTTP/HTTPS load testing and benchmarking tool supporting http uploading file.

Originated from [gobench](https://github.com/cmpxchg16/gobench). 

    bingoo@localhost ~/G/h/gobench> ./gobench -up http://127.0.0.1:8811/upload  -c 100 -p /tmp/fix5242880  -r 1000
    Dispatching 100 clients
    Waiting for results...

    Requests:                           100000 hits
    Successful requests:                 99651 hits
    Network failed:                        349 hits
    Bad requests failed (!2xx):              0 hits
    Successful requests rate:              153 hits/sec
    Read throughput:                     11498 bytes/sec
    Write throughput:                803968443 bytes/sec
    Test time:                             650 sec

gobench usage:

    bingoo@localhost ~/G/h/gobench> ./gobench
    Usage of ./gobench:
    -auth string
            Authorization header
    -c int
            Number of concurrent clients (default 100)
    -d string
            HTTP POST data file path
    -fn string
            Upload file name (default "file")
    -fp string
            HTTP upload file path
    -fr
            Upload random png images by file upload
    -k	Do HTTP keep-alive (default true)
    -r int
            Number of requests per client (default -1)
    -t int
            Period of time (in seconds) (default -1)
    -tr int
            Read timeout (in milliseconds) (default 5000)
    -tw int
            Write timeout (in milliseconds) (default 5000)
    -u string
            URL list (comma seperated)
    -uf string
            URL's file path (line seperated)
    -ur
            select url in Rand-Robin  (default true)
