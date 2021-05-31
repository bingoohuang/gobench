```sh
$ http-raw -addr localhost:5003 -input "OPTIONS * HTTP/1.1\nHost: localhost:5003"
Request: === start ===
"OPTIONS * HTTP/1.1\r\nHost: localhost:5003\r\n\r\n"

Request: === end ===
HTTP/1.1 200 OK
Content-Length: 0
Date: Mon, 31 May 2021 06:37:21 GMT
```
