#!/bin/bash

cd test/molecule-role/molecule/compose/files/
docker-compose down
docker volume prune -f
docker-compose up -d --remove-orphans
docker-compose exec kafka bash
