install:
	/usr/local/go/bin/go build -o mini_raw
	# cp ./mini_raw /home/wlcsm/.local/bin/mini_raw
	# cp ./driver.sh /home/wlcsm/.local/bin/mini
clean:
	rm mini
