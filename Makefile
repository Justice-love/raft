.PHONY: proto
build: clean proto
	@echo "Compiling source"
	@mkdir -p build
	go build  -o build/net cmd/cmd.go

serve: build
	@echo "Starting Server"
	./build/net

clean:
	@echo "Cleaning up workspace"
	@rm -f proto/message.pb.go
	@rm -rf build

proto:
	@go generate proto/proto.go