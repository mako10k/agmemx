PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
BASH_COMPLETION_DIR = $(PREFIX)/share/bash-completion/completions
MANDIR = $(PREFIX)/share/man/man1

.PHONY: build install uninstall

build:
	go build -o bin/agmemx ./cmd/agmemx

install: build
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 0755 bin/agmemx "$(DESTDIR)$(BINDIR)/agmemx"
	install -d "$(DESTDIR)$(MANDIR)"
	install -m 0644 docs/agmemx.1 "$(DESTDIR)$(MANDIR)/agmemx.1"
	if [ -f completions/agmemx.bash ]; then \
		install -d "$(DESTDIR)$(BASH_COMPLETION_DIR)"; \
		install -m 0644 completions/agmemx.bash "$(DESTDIR)$(BASH_COMPLETION_DIR)/agmemx"; \
	fi

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/agmemx" \
		"$(DESTDIR)$(BASH_COMPLETION_DIR)/agmemx" \
		"$(DESTDIR)$(MANDIR)/agmemx.1"
