#!/bin/bash

curl --include --no-buffer -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: 1" http://localhost:1337/ -o /dev/null
