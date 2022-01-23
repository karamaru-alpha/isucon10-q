SHELL=/bin/bash
api-server/%: ## api-server/${lang}docker-compose up with mysql and api-server
	docker-compose -f docker-compose/$(shell basename $@).yaml down -v
	docker-compose -f docker-compose/$(shell basename $@).yaml up --build mysql api-server

isuumo/%: ## isuumo/${lang} docker-compose up with mysql and api-server frontend nginx
	docker-compose -f docker-compose/$(shell basename $@).yaml down -v
	docker-compose -f docker-compose/$(shell basename $@).yaml up --build mysql api-server nginx frontend

APP:=isuumo
APP_PATH:=/home/isucon/isuumo/webapp
GO_PATH:=/home/isucon/local/go/bin/go
DB_HOST:=127.0.0.1
DB_PORT:=3306
DB_USER:=isucon
DB_PASS:=isucon
DB_NAME:=isuumo
MYSQL_LOG:=/var/log/mysql/slow-query.log
MYSQL_ERR:=/var/log/mysql/error.log
NGINX_LOG:=/var/log/nginx/access.log
NGINX_ERR:=/var/log/nginx/error.log
GO_LOG:=/var/log/go.log

MAIN_SERVER:=isu1
DB_SERVER:=isu1
# APP_SERVER:=isu3


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
	sudo sed -i -e "s/slow_count[ \f\n\r\t]*=.*/slow_count = 10/" kataribe.toml
	sudo sed -i -e "s/show_stddev[ \f\n\r\t]*=.*/show_stddev = false/" kataribe.toml
	sudo sed -i -e "s/show_status_code[ \f\n\r\t]*=.*/show_status_code = false/" kataribe.toml
	sudo sed -i -e "s/show_bytes[ \f\n\r\t]*=.*/show_bytes = false/" kataribe.toml
	sudo sed -i -e "s/percentiles[ \f\n\r\t]*=.*/percentiles = []/" kataribe.toml
	sudo rm README.md 2> /dev/null
	sudo rm LICENSE 2> /dev/null
	gh auth login
# GitHub.com -> SSH -> /home/isucon/.ssh/id_rsa.pub -> Paste an authentication token -> https://github.com/settings/tokens

.PHONY: before
before:
	ssh $(MAIN_SERVER) "\
		cd $(APP_PATH);\
		git stash;\
		git pull origin main;\
		sudo cp my.cnf /etc/mysql/my.cnf;\
		sudo cp nginx.conf /etc/nginx/nginx.conf;\
		sudo cp $(APP).conf /etc/nginx/sites-enabled/$(APP).conf;\
		(cd go && $(GO_PATH) mod tidy);\
		(cd go && $(GO_PATH) build -o $(APP));\
		sudo cp /dev/null $(MYSQL_LOG);\
		sudo cp /dev/null $(MYSQL_ERR);\
		sudo cp /dev/null $(NGINX_LOG);\
		sudo cp /dev/null $(NGINX_ERR);\
		sudo cp /dev/null $(GO_LOG);\
		sudo systemctl restart nginx;\
		sudo systemctl restart mysql;\
		sudo systemctl restart $(APP).go.service;\
	"

# DB 切り分け後に有効化
# .PHONY: before
# before:
# 	ssh $(MAIN_SERVER) "\
# 		cd $(APP_PATH);\
# 		git stash;\
# 		git pull origin main;\
# 		sudo cp nginx.conf /etc/nginx/nginx.conf;\
# 		sudo cp $(APP).conf /etc/nginx/sites-enabled/$(APP).conf;\
# 		(cd go && $(GO_PATH) mod tidy);\
# 		(cd go && $(GO_PATH) build -o $(APP));\
# 		sudo rm $(MYSQL_LOG) 2> /dev/null;\
# 		sudo rm $(NGINX_LOG) 2> /dev/null;\
# 		sudo touch $(NGINX_LOG);\
# 		sudo rm $(GO_LOG) 2> /dev/null;\
# 		sudo touch $(GO_LOG);\
# 		sudo chmod 0666 $(GO_LOG);\
# 		sudo cp /dev/null /var/log/nginx/error.log;\
# 		sudo systemctl restart nginx;\
# 		sudo systemctl stop mysql;\
# 		sudo systemctl restart $(APP).go.service;\
# 	"
# 	ssh $(DB_SERVER) "\
# 		cd $(APP_PATH);\
# 		git stash;\
# 		git pull origin main;\
# 		sudo cp my.cnf /etc/mysql/my.cnf;\
# 		sudo rm $(MYSQL_LOG) 2> /dev/null;\
# 		sudo touch $(MYSQL_LOG);\
# 		sudo chown -R mysql $(MYSQL_LOG);\
# 		sudo systemctl stop nginx;\
# 		sudo systemctl restart mysql;\
# 		sudo systemctl stop $(APP).go.service;\
# 	"

.PHONY: sql
sql:
	mysql -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER) -p$(DB_PASS) $(DB_NAME)

.PHONY: slow
slow:
	ssh $(DB_SERVER) 'sudo ./slow.sh $(MYSQL_LOG)'

.PHONY: kataru
kataru:
	ssh $(MAIN_SERVER) "sudo cat $(NGINX_LOG) | kataribe -f $(APP_PATH)/kataribe.toml"

.PHONY: log
log:
	ssh $(MAIN_SERVER) "sudo cat $(GO_LOG)"

.PHONY: bench
bench:
	ssh $(MAIN_SERVER) "cd /home/isucon/isuumo/bench && ./bench -target-url http://localhost:80"

.PHONY: fetch
fetch:
	ssh $(MAIN_SERVER) "cd $(APP_PATH) && git fetch origin main && git reset --hard origin/main"
