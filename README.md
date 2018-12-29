# http-upload-benchmark
HTTP upload server benchmark in few languages/frameworks and gobench tool

# Golang upload server

## Building
1. `cd go-upload-server`
1. build: `go build -o go-upload-server.bin`
2. create 5MiB sample file then startup upload server: `./go-upload-server -impl nethttp -sampleFileSize 5242880`
3. benchmarking: `cd scripts; ./start-jmeter-workbench.sh`

## Result

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

