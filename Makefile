mini: mini.go
install:
	/usr/local/go/bin/go build
	cp ./mini /home/wlcsm/.local/bin/mini
clean:
	rm mini
