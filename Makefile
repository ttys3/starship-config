DIR:=$(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))

install:
	ln -sf $(DIR)/starship.toml ~/.config/starship.toml
