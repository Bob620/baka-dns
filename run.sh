#!/bin/sh

#module add soft/go
rm ./redis-server
wget http://download.redis.io/redis-stable.tar.gz
tar -xf redis-stable.tar.gz
rm redis-stable.tar.gz
cd ./redis-stable || exit
make
cp ./src/redis-server ../
cd .. || exit
rm -fr ./redis-stable
./redis-server redis.conf