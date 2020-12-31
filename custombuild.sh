#!/usr/bin/env bash
ENDPOINT="$1"

[[ ! $ENDPOINT ]] && {
	echo "Missing endpoint. Usage: ./custombuild.sh https://endpoint.to.discord"
	exit 1
}

go build -ldflags="-X 'github.com/diamondburned/arikawa/api.BaseEndpoint=$ENDPOINT'"
