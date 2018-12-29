# http-upload-benchmark
HTTP upload server benchmark in few languages/frameworks and gobench tool

# Golang upload server

## Building
1. `cd go-upload-server`
1. build: `go build -o go-upload-server.bin`
2. create 5MiB sample file then startup upload server: `./go-upload-server -impl nethttp -sampleFileSize 5242880`
3. benchmarking: `cd scripts; ./start-jmeter-workbench.sh`

## Result

Environment:

>>  bingoo@bingodeMacBook-Pro ~> system_profiler SPHardwareDataType
>>  Hardware:
>>  
>>      Hardware Overview:
>>  
>>        Model Name: MacBook Pro
>>        Model Identifier: MacBookPro15,1
>>        Processor Name: Intel Core i7
>>        Processor Speed: 2.6 GHz
>>        Number of Processors: 1
>>        Total Number of Cores: 6
>>        L2 Cache (per Core): 256 KB
>>        L3 Cache: 9 MB
>>        Memory: 16 GB


1. net/http with 5 mib random context file upload without temp file.

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 40000     | 120     | 112    | 186      | 213      | 274      | 15  | 441 | 0.000%  | 326.69596  | 23.93           | 1672831.02  | 
| 总体     | 40000     | 120     | 112    | 186      | 213      | 274      | 15  | 441 | 0.000%  | 326.69596  | 23.93           | 1672831.02  | 

2. fasthttp with 5 mib random context file upload without temp file.

| Label  | # Samples | Average | Median | 90% Line | 95% Line | 99% Line | Min | Max | Error % | Throughput | Received KB/sec | Sent KB/sec | 
|--------|-----------|---------|--------|----------|----------|----------|-----|-----|---------|------------|-----------------|-------------| 
| HTTP请求 | 40000     | 119     | 111    | 177      | 207      | 277      | 19  | 455 | 0.000%  | 330.00033  | 29.97           | 1689750.90  | 
| 总体     | 40000     | 119     | 111    | 177      | 207      | 277      | 19  | 455 | 0.000%  | 330.00033  | 29.97           | 1689750.90  | 



# gobench

It is a HTTP/HTTPS load testing and benchmarking tool supporting http uploading file.

Originated from [gobench](https://github.com/cmpxchg16/gobench). 



> bingoo@localhost ~/G/h/gobench> ./gobench -up http://127.0.0.1:8811/upload  -c 100 -p /tmp/fix5242880  -r 1000
> Dispatching 100 clients
> Waiting for results...
> 
> Requests:                           100000 hits
> Successful requests:                 99651 hits
> Network failed:                        349 hits
> Bad requests failed (!2xx):              0 hits
> Successful requests rate:              153 hits/sec
> Read throughput:                     11498 bytes/sec
> Write throughput:                803968443 bytes/sec
> Test time:                             650 sec


> bingoo@localhost ~/G/h/gobench> ./gobench
> Usage of ./gobench:
>   -auth string
>     	Authorization header
>   -c int
>     	Number of concurrent clients (default 100)
>   -d string
>     	HTTP POST data file path
>   -f string
>     	URL's file path (line seperated)
>   -k	Do HTTP keep-alive (default true)
>   -r int
>     	Number of requests per client (default -1)
>   -t int
>     	Period of time (in seconds) (default -1)
>   -tr int
>     	Read timeout (in milliseconds) (default 5000)
>   -tw int
>     	Write timeout (in milliseconds) (default 5000)
>   -u string
>     	URL
>   -up string
>     	HTTP upload file path
