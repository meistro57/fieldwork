BINARY=bin/fieldwork

.PHONY: build service dig query status clean

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/fieldwork

service:
	docker compose up --build marker_service redis qdrant

dig:
	go run ./cmd/fieldwork dig $(ARGS)

query:
	go run ./cmd/fieldwork query "$(QUERY)" $(ARGS)

status:
	go run ./cmd/fieldwork status $(ARGS)

clean:
	rm -rf bin
