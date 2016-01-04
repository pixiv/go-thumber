#!/bin/sh -eux

if [ "$1" != "" ]; then
  cd /tmp
  wget http://downloads.sourceforge.net/project/libjpeg-turbo/1.3.1/libjpeg-turbo-official_$1_amd64.deb
  sudo dpkg -i libjpeg-turbo-official_$1_amd64.deb
  find /opt/libjpeg-turbo/lib64
  find /opt/libjpeg-turbo/include
fi
