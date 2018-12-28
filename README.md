# http-upload-benchmark
HTTP upload server benchmark in few languages/frameworks and gobench tool

## Golang upload server
1. `cd go-upload-server`
1. build: `go build -o go-upload-server.bin`
2. create 5MiB sample file then startup upload server: `./go-upload-server -impl nethttp -sampleFileSize 5242880`
3. benchmarking: `cd scripts; ./start-jmeter-workbench.sh`
