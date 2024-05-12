BINARY_NAME = deviceregapi
.DEFAULT_GOAL := run
export MONGO_SRVR=aqua.eensymachines.in:32701
export MONGO_USER=eensyaquap-dev
export MONGO_PASS=33n5y+4dm1n
# Cheifly signifies how the secrets would be read
# Incases of developmenti environment we read from environment
export APP_MODE=DEV
export MONGO_DB_NAME=aquaponics
build:
	go build -o ./${BINARY_NAME} . 

run: build

	./${BINARY_NAME}
	
deploy:
	docker buildx build --push -t kneerunjun/webapi-devicereg:v0.0.3 .
	kubectl delete -f ./k8s/gin.deploy.yml
	kubectl apply -f ./k8s/gin.deploy.yml
	kubectl get pods --watch -owide