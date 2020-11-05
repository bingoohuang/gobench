# tools list

1. [k6 A modern load testing tool, using Go and JavaScript](https://github.com/loadimpact/k6)
1. [ali HTTP upload server benchmark in few languages/frameworks and gobench tool](https://github.com/nakabonne/ali)
   ![Screenshot](https://raw.githubusercontent.com/nakabonne/ali/master/images/demo.gif)
1. [Vegeta HTTP load testing tool and library.](https://github.com/tsenart/vegeta)

   欣赏点:

   If you are a happy user of iTerm, you can integrate vegeta with jplot using jaggr to plot a vegeta report in real-time in the comfort of your terminal:

   ```bash
   echo 'GET http://localhost:8080' | \
       vegeta attack -rate 5000 -duration 10m | vegeta encode | \
       jaggr @count=rps \
             hist\[100,200,300,400,500\]:code \
             p25,p50,p95:latency \
             sum:bytes_in \
             sum:bytes_out | \
       jplot rps+code.hist.100+code.hist.200+code.hist.300+code.hist.400+code.hist.500 \
             latency.p95+latency.p50+latency.p25 \
             bytes_in.sum+bytes_out.sum
   ```

   ![](https://i.imgur.com/ttBDsQS.gif)

1. [An HTTP performance testing tool written in GoLang](https://github.com/arham-jain/gonce)
1. [Modern cross-platform HTTP load-testing tool written in Go](https://github.com/rogerwelin/cassowary)
   > Cassowary 是一个现代的 http / s、直观的跨平台负载测试工具，Go 开发，用于开发人员、测试人员和系统管理员。 食火鸡从很棒的项目中获得灵感，比如 k6，ab & httpstat。
1. [ultron a http load testing tool in go](https://github.com/qastub/ultron), FROM [聊聊 ab、wrk、JMeter、Locust 这些压测工具的并发模型差别](https://mp.weixin.qq.com/s/0ZPHT1MXBP7EWjVmv-H5Wg)
1. [Open source load testing tool review 2020](https://k6.io/blog/comparing-best-open-source-load-testing-tools)

   | TOOL                        | APACHEBENCH        | ARTILLERY               | DRILL                 | GATLING             |
   | --------------------------- | ------------------ | ----------------------- | --------------------- | ------------------- |
   | Created by                  | Apache foundation  | Shoreditch Ops LTD      | Ferran Basora         | Gatling Corp        |
   | License                     | Apache 2\.0        | MPL2                    | GPL3                  | Apache 2\.0         |
   | Written in                  | C                  | NodeJS                  | Rust                  | Scala               |
   | Scriptable                  | No                 | Yes: JS                 | No                    | Yes: Scala          |
   | Multithreaded               | No                 | No                      | Yes                   | Yes                 |
   | Distributed load generation | No                 | No \(Premium\)          | No                    | No \(Premium\)      |
   | Website                     | httpd\.apache\.org | artillery\.io           | github\.com/fcsonline | gatling\.io         |
   | Source code                 | svn\.apache\.org   | github\.com/artilleryio | github\.com/fcsonline | github\.com/gatling |

   | TOOL                        | HEY                | JMETER              | K6                | LOCUST          |
   | --------------------------- | ------------------ | ------------------- | ----------------- | --------------- |
   | Created by                  | Jaana B Dogan      | Apache foundation   | Load Impact       | Jonathan Heyman |
   | License                     | Apache 2\.0        | Apache 2\.0         | AGPL3             | MIT             |
   | Written in                  | Go                 | Java                | Go                | Python          |
   | Scriptable                  | No                 | Limited \(XML\)     | Yes: JS           | Yes: Python     |
   | Multithreaded               | Yes                | Yes                 | Yes               | No              |
   | Distributed load generation | No                 | Yes                 | No \(Premium\)    | Yes             |
   | Website                     | github\.com/rakyll | jmeter\.apache\.org | k6\.io            | locust\.io      |
   | Source code                 | github\.com/rakyll | github\.com/apache  | loadimpact@github | locustio@github |

   | TOOL                        | HEY           | JMETER                | K6             | LOCUST               |
   | --------------------------- | ------------- | --------------------- | -------------- | -------------------- |
   | Created by                  | Jeff Fulmer   | Nicolas Niclausse     | Tomás Senart   | Will Glozer          |
   | License                     | GPL3          | GPL2                  | MIT            | Apache 2\.0 modified |
   | Written in                  | C             | Erlang                | Go             | C                    |
   | Scriptable                  | No            | Limited \(XML\)       | No             | Yes: Lua             |
   | Multithreaded               | Yes           | Yes                   | Yes            | Yes                  |
   | Distributed load generation | No            | Yes                   | Limited        | No                   |
   | Website                     | joedog\.org   | erland\-projects\.org | tsenart@github | wg@github            |
   | Source code                 | JoeDog@github | processone@github     | tsenart@github | wg@github            |

   ![image](https://user-images.githubusercontent.com/1940588/76588218-73d6bb00-6521-11ea-8eb6-f22db5aeab97.png)

   ![image](https://user-images.githubusercontent.com/1940588/76588260-86e98b00-6521-11ea-9eec-b0a4f1ae4d80.png)

   k6 rulez!

   Or, uh, well it does, but most of these tools have something going for them. They are simply good in different situations.

   I don't really like Gatling, but understand why others like it in the "I need a more modern Jmeter" use case.
   I like Hey in the "I need a simple command-line tool to hit a single URL with some traffic" use case.
   I like Vegeta in the "I need a more advanced command-line tool to hit some URLs with traffic" use case.
   I don't like Jmeter much at all, but guess non-developers may like it in the "We really want a Java-based tool/GUI tool that can do everything" use case.
   I like k6 (obviously) in the "automated testing for developers" use case.
   I like Locust in the "I'd really like to write my test cases in Python" use case.
   I like Wrk in the "just swamp the server with tons of requests already!" use case.
   Then we have a couple of tools that seem best avoided:

   Siege is just old, strange and unstable and the project seems almost dead.
   Artillery is super-slow, measures incorrectly and the open source version doesn't seem to be moving forward much.
   Drill is accelerating global warming.
