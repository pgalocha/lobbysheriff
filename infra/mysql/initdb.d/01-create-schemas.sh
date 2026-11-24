#!/bin/bash
# Cria um schema e um utilizador por serviço (Database per Service).
# Cada utilizador só tem privilégios sobre o seu próprio schema.
#
# Corre apenas na PRIMEIRA inicialização do volume do MySQL. Para voltar a
# correr: docker compose down -v (apaga os dados locais).
set -euo pipefail

create_service_db() {
  local db="$1" user="$2" password="$3"
  # Escapar valores vindos do .env: ` em identificadores, ' em literais.
  # NO_BACKSLASH_ESCAPES faz com que \ seja guardado tal como está.
  db="${db//\`/\`\`}"
  user="${user//\'/\'\'}"
  password="${password//\'/\'\'}"
  echo "initdb: a criar schema '${db}' e utilizador '${user}'"
  mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" <<SQL
SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES');
CREATE DATABASE IF NOT EXISTS \`${db}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER IF NOT EXISTS '${user}'@'%' IDENTIFIED BY '${password}';
GRANT ALL PRIVILEGES ON \`${db}\`.* TO '${user}'@'%';
SQL
}

create_service_db "${STATS_DB_NAME}" "${STATS_DB_USER}" "${STATS_DB_PASSWORD}"
create_service_db "${PLAYERS_DB_NAME}" "${PLAYERS_DB_USER}" "${PLAYERS_DB_PASSWORD}"

mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "FLUSH PRIVILEGES;"
