EXEC = cermanager
GO = go
BINPATH = bin
TARGET = $(BINPATH)/$(EXEC)

all: $(TARGET)

$(TARGET):
	$(GO) build -o $(BINPATH)/$(EXEC) cmd/cermanager/*

install: $(TARGET)
	cp $(TARGET) /usr/local/bin/

.PHONY : clean
clean:
	rm -rf $(BINPATH)