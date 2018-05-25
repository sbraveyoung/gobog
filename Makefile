
all:gobog

gobog:clean
	go build

.PHONY:clean
	go clean

start:gobog
	nohup ./gobog >debug.log 2>&1 &

restart:stop gobog
	nohup ./gobog >debug.log 2>&1 &

stop:
	kill -9 `ps aux | grep gobog | grep -v "grep" | awk '{print $$2}'`
