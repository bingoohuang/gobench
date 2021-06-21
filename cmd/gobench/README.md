1. 类似的压力测试工具
    - [httpfuzz is a fast HTTP fuzzer written in Go inspired by Burp Intruder](https://github.com/JonCooperWorks/httpfuzz)
    - [A high-performance HTTP benchmarking tool with real-time web UI and terminal displaying](https://github.com/six-ddc/plow)
    - [codesenberg/bombardier](https://github.com/codesenberg/bombardier)
    - [denji/awesome-http-benchmark](https://github.com/denji/awesome-http-benchmark)
    - [Modern cross-platform HTTP load-testing tool written in Go](https://github.com/rogerwelin/cassowary)
    - [go wrk](https://github.com/adjust/go-wrk)
    - [httpit](https://github.com/gonetx/httpit)

1. 随机读取es性能测试

```shell
[root@fs04-192-168-126-5 bingoo]# gobench -badger es-badger-db:1000000 -l ":9202/license/_search?routing=@v&q=holderIdentityNum:@v" -ok 'hits_total >= 1' -n1000000
Dispatching 100 goroutines at 2021-06-10 17:17:25.804
1000000 / 1000000 [-----------------------------] 100.00% 11659 p/s

Total requests:				1000000 hits
OK    requests:				1000000 hits
Network failed(NF):			0 hits
Bad requests(!2xx/hits_total >= 1)(BF):	0 hits
OK requests rate:			11631.963 hits/sec
Read throughput:			15 MiB/sec
Write throughput:			2.6 MiB/sec
Test time:				1m25.97s(2021-06-10 17:17:25.804-17:18:51.774)
Max X-Gobench-Seq:			1000000
[root@fs04-192-168-126-5 bingoo]# du -sh es-badger-db/
18G	es-badger-db/
[root@fs04-192-168-126-5 bingoo]# pwd
/home/bingoo
```
