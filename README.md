# starship-config

starship config with custom modules (for Linux amd64 only)

## Installation

1. setup starship

```shell
git clone https://github.com/ttys3/starship-config
cd starship-config
make install

# custom module helper tools installation
sudo curl -fL https://github.com/ttys3/starship-config/releases/download/v0.1.0/os-icon  -o /usr/local/bin/os-icon
# Make the binary executable:
sudo chmod a+x /usr/local/bin/os-icon

```

2. config zsh to use starship prompt

just add `eval "$(starship init zsh)"` to your `~/.zshrc` file

---------------------------------------------

## Screenshot

![screenshot](https://user-images.githubusercontent.com/41882455/120683401-2781ed00-c4d0-11eb-8937-f7265d66dcdd.png)
