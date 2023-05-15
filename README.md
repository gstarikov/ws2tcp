# ws2tcp
ws2tcp relay
- device - tcp server that listen tcp socket and on request will rent endlerr responce stream with variadbe payload size. from 700 up to 1500 bytes 
- relay - http websocket server, on any incoming request will try to upgrade to websocket, send request to device and reply all tcp incoming data to websocket. also in does crc64 control and json marshall/ unmarshall
## how to use it
###  in one terminal window start device imitator
```shell
cd cmd/device 
go build
INTERVAL=5ms ./device
```
### in second terminal window  start relay imitator
```shell
cd cmd/relay 
go build
./relay
```
### in third terminal sent test request
```shell
curl --include --no-buffer -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: 1" http://localhost:1337/ -o -
```
### take a look into second window and wait for errors :) 

# at codebase level 
- check [/internal/relay/accumulator.go]
