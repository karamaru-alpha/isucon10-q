SHELL=/bin/bash
api-server/%: ## api-server/${lang}docker-compose up with mysql and api-server
	docker-compose -f docker-compose/$(shell basename $@).yaml down -v
	docker-compose -f docker-compose/$(shell basename $@).yaml up --build mysql api-server

isuumo/%: ## isuumo/${lang} docker-compose up with mysql and api-server frontend nginx
	docker-compose -f docker-compose/$(shell basename $@).yaml down -v
	docker-compose -f docker-compose/$(shell basename $@).yaml up --build mysql api-server nginx frontend

APP:=isuumo
DB_HOST:=127.0.0.1
DB_PORT:=3306
DB_USER:=isucon
DB_PASS:=isucon
DB_NAME:=isuumo
MYSQL_LOG:=/var/log/mysql/slow-query.log
NGINX_LOG:=/var/log/nginx/access.log
GO_LOG:=/var/log/go.log

.PHONY: setup
setup:
	curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
	sudo apt update
	sudo apt install -y percona-toolkit git unzip gh
	git init
	git config --global user.name karamaru-alpha
	git config --global user.email mrnk3078@gmail.com
	git config credential.helper store
	wget https://github.com/matsuu/kataribe/releases/download/v0.4.1/kataribe-v0.4.1_linux_amd64.zip -O kataribe.zip
	unzip -o kataribe.zip
	sudo mv kataribe /usr/local/bin/
	sudo chmod +x /usr/local/bin/kataribe
	sudo rm kataribe.zip
	kataribe -generate
	sudo rm README.md 2> /dev/null
	sudo rm LICENSE 2> /dev/null
	gh auth login
# GitHub.com -> SSH -> /home/isucon/.ssh/id_rsa.pub -> Paste an authentication token -> https://github.com/settings/tokens

.PHONY: before
before:
# 同期
	git stash
	git pull origin main
	sudo cp my.cnf /etc/mysql/my.cnf
	sudo cp nginx.conf /etc/nginx/nginx.conf
	sudo cp $(APP).conf /etc/nginx/sites-enabled/$(APP).conf
# ビルド
	(cd go && go mod tidy)
	(cd go && go build -o $(APP))
# 掃除
	sudo rm $(MYSQL_LOG) 2> /dev/null
	sudo touch $(MYSQL_LOG)
	sudo chown -R mysql $(MYSQL_LOG)
	sudo rm $(NGINX_LOG) 2> /dev/null
	sudo touch $(NGINX_LOG)
	sudo rm $(GO_LOG) 2> /dev/null
	sudo touch $(GO_LOG)
	sudo chmod 0666 $(GO_LOG)
	sudo cp /dev/null /var/log/nginx/error.log
# 起動
	sudo systemctl restart nginx
	sudo systemctl restart mysql
	sudo systemctl restart $(APP).go.service

.PHONY: before-db
before-db:
	git stash
	git pull origin main
	sudo cp my.cnf /etc/mysql/my.cnf
	sudo rm $(MYSQL_LOG) 2> /dev/null
	sudo touch $(MYSQL_LOG)
	sudo chown -R mysql $(MYSQL_LOG)
	sudo systemctl restart mysql
	sudo systemctl stop nginx
	sudo systemctl stop $(APP).go.service

.PHONY: sql
sql:
	mysql -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER) -p$(DB_PASS) $(DB_NAME)

.PHONY: slow
slow:
	sudo pt-query-digest $(MYSQL_LOG)

.PHONY: kataru
kataru:
	sudo cat $(NGINX_LOG) | kataribe -f ./kataribe.toml

.PHONY: log
log:
	sudo cat $(GO_LOG)

.PHONY: bench
bench:
	(cd ../bench && ./bench -target-url http://localhost:80)
