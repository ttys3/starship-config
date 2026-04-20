DIR:=$(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))

.PHONY: install install-config install-git-remote-visibility

install: install-config install-git-remote-visibility

install-config:
	ln -sf $(DIR)/starship.toml ~/.config/starship.toml

install-git-remote-visibility:
	$(MAKE) -C git-remote-visibility install
