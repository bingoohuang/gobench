# gobench

a HTTP/HTTPS load testing and benchmarking tool supporting http uploading. Originated
from [gobench](https://github.com/cmpxchg16/gobench).

```bash
$ gobench http://127.0.0.1:5003/api/demo  
Dispatching 100 goroutines at 2021-03-24 23:25:57.176
Waiting for results...

Total Requests:                 664004 hits
Successful requests:            664004 hits
Network failed:                 0 hits
Bad requests(!2xx):             0 hits
Successful requests rate:       66391 hits/sec
Read throughput:                19 MiB/sec
Write throughput:               6.0 MiB/sec
Test time:                      10.001s(2021-03-24 23:25:57.176-23:26:07.177)


$ gobench -h
Usage: gobench [options...] url1[,url2...]
Options:
  -l               URL list (comma separated), or @URL's file path (line separated)
  -X               HTTP method(GET, POST, PUT, DELETE, HEAD, OPTIONS and etc)
  -c               Number of connections (default 100)
  -n               Number of total requests
  -t               Number of concurrent goroutines (default 100)
  -r               Number of requests per goroutine
  -d               Duration of time (eg 10s, 10m, 2h45m) (10s if no total requests or per-goroutine-requests set)
  -p               Print something. 0: Print http response; 1: with extra newline; x.log: log file
  -proxy           Proxy url, like socks5://127.0.0.1:1080, http://127.0.0.1:1080
  -P               POST data, use @a.json for a file
  -c.type          Content-Type, eg, json, plain, or other full name
  -auth            Authorization header
  -k               HTTP keep-alive (default true)
  -ok              Condition like 'status == 200' for json output
  -image           Upload random images by file upload, png/jpg
  -i.size          Upload fixed img size (eg. 44kB, 17MB)
  -u.file          Upload file path
  -u.field         Upload field name (default "file")
  -r.timeout       Read timeout (like 5ms,10ms,10s) (default 5s)
  -w.timeout       Write timeout (like 5ms,10ms,10s) (default 5s)
  -cpus            Number of used cpu cores. (default for current machine is 12 cores)
  -think           Think time, eg. 1s, 100ms, 100-200ms and etc. (unit ns, us/µs, ms, s, m, h)
  -v               Print version
  -weed            Weed master URL, like http://127.0.0.1:9333
  -pprof           Profile pprof address, like localhost:6060
```

## Features

- [x] [seeweedfs](https://github.com/chrislusf/seaweedfs) bench support
  like `gobench -weed=http://192.168.126.5:3333 -u.file=./weedfiles -n=10000 -think 100-500ms -p 1`

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

post json evaluating:

```sh
$ gobench -l ":5003/license/docs?routing=@身份证_keep" -P @testdata/es.json -n1 -p1
Dispatching 1 goroutines at 2021-06-07 18:46:53.655
2021-06-07 18:46:53.656 URL: http://127.0.0.1:5003/license/docs?routing=325981200805085623
2021-06-07 18:46:53.656 POST: { "idCode": "332a7c20-3704-49f4-a91c-56f605f2c7d1", "licenseCode": "236a85a4-a865-439e-91b0-6d5e81d5ec89", "implementCode": "210109732810121411719", "name": "淩哂证", "licenseStatus": "ISSUED", "holderName": [ "寿幗犮" ], "holderIdentityType": "11", "holderIdentit [ "325981200805085623" ], "dataType": "INTEGRATION", "bizType": "LICENSE_ISSUE", "bizStatus": "PASSED", "implementOrgCode": "971554898", "issueOrgName": "惠州市公安局某某分局", "issueOrgCode": "091933041257201987", "divisionCode": "670066316492", "auditTime": "2063-05-15T08:18:45Z", "issueDat03-05T12:52:17Z", "areaCode": "937212", "trustLevel": "T", "id": "5fca0c02-3942-41d7-a3f9-fc4ae3883c8c", "createdBy": "甘墨錹", "createdDate": "2026-07T18:46:53Z", "lastModifiedBy": "毕亇鳴", "lastModifiedDate": "2018-07-21T08:00:49Z" }
2021-06-07 18:46:53.657 [√] [200] HTTP/1.1 200 OK Date: Mon, 07 Jun 2021 10:46:53 GMT Content-Type: application/json; charset=utf-8 Content-Length: 8 X-Gobench-Seq: 1 X-Gobench-Time: 2021-06-07 18:46:53.656 {"OK":1}
```

`@身份证_keep`, the variable name ending with `_keep` will be remembered and reused in the later substitution.

es.json

```json
{
  "idCode": "@uuid",
  "licenseCode": "@uuid",
  "implementCode": "@regex([1-9][0-9]{20})",
  "name": "@{XX}证",
  "licenseStatus": "ISSUED",
  "holderName": [
    "@姓名"
  ],
  "holderIdentityType": "@regex([0-9]{2})",
  "holderIdentityNum": [
    "@身份证_keep"
  ],
  "dataType": "INTEGRATION",
  "bizType": "LICENSE_ISSUE",
  "bizStatus": "PASSED",
  "implementOrgCode": "@regex([0-9]{9})",
  "issueOrgName": "@发证机关",
  "issueOrgCode": "@regex([0-9]{18})",
  "divisionCode": "@regex([0-9]{12})",
  "auditTime": "@random_time(yyyy-MM-ddTHH:mm:ssZ)",
  "issueDate": "@random_time(yyyy-MM-ddTHH:mm:ssZ)",
  "areaCode": "@regex([0-9]{6})",
  "trustLevel": "@regex([A-Z])",
  "id": "@uuid",
  "createdBy": "@姓名",
  "createdDate": "@random_time(now, yyyy-MM-ddTHH:mm:ssZ)",
  "lastModifiedBy": "@姓名",
  "lastModifiedDate": "@random_time(yyyy-MM-ddTHH:mm:ssZ)"
}
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

