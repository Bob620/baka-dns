#!/bin/sh

go || (module add soft/go)
wget http://download.redis.io/redis-stable.tar.gz
tar -xf redis-stable.tar.gz
rm redis-stable.tar.gz
cd ./redis-stable || exit
make
cp ./src/redis-server ../
cd .. || exit
rm -fr ./redis-stable
go build main.go