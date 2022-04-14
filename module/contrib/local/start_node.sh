#!/bin/bash
set -eu

gravityd start --rpc.laddr tcp://0.0.0.0:26658 --trace --log_level="main:info,state:debug,*:error"