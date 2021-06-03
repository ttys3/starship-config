# starship-config

starship config with custom modules

## Installation

1. setup starship

```shell
git clone https://github.com/ttys3/starship-config
cd starship-config
make install

# custom module helper tools installation
# os-icon.linux_amd64 is for Linux amd64
# select the correct binary for your os here: https://github.com/ttys3/starship-config/releases/tag/v0.2.0
sudo curl -fL https://github.com/ttys3/starship-config/releases/download/v0.2.0/os-icon.linux_amd64  -o /usr/local/bin/os-icon
# Make the binary executable:
sudo chmod a+x /usr/local/bin/os-icon

```

2. config zsh to use starship prompt

just add `eval "$(starship init zsh)"` to your `~/.zshrc` file

---------------------------------------------

## Screenshot

![screenshot](https://user-images.githubusercontent.com/41882455/120683401-2781ed00-c4d0-11eb-8937-f7265d66dcdd.png)


![2021-06-04_01-51](https://user-images.githubusercontent.com/41882455/120690121-64051700-c4d7-11eb-92eb-f3a895d7eac7.png)
