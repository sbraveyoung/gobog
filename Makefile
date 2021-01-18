all:build

.PHONY:clean

build:clean
	go build -o gobog src/main.go

release:build
	mkdir release/bin
	cp -r conf themes release
	mv gobog release/bin

start:
	nohup ./gobog >debug.log 2>&1 &

stop:
	kill -9 `ps aux | grep gobog | grep -v "grep" | awk '{print $$2}'`

restart:stop,start
