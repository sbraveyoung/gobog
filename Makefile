all: build

.PHONY: clean build release start stop restart

clean:
	rm -f gobog
	rm -rf release

build: clean
	go build -o gobog src/main.go

release: build
	mkdir -p release/bin
	cp -r conf themes release
	mv gobog release/bin

start:
	nohup ./gobog >debug.log 2>&1 &

stop:
	kill -9 `ps aux | grep gobog | grep -v "grep" | awk '{print $$2}'`

restart: stop start
