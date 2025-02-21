#!/bin/bash
# Processa a resposta negativa do cliente à oferta de DCC via endpoint /accept-dcc
# Salve este arquivo como reject_dcc.sh e torne-o executável com:
# chmod +x reject_dcc.sh
#
# Altere "order-1" para o OrderID retornado na criação do pedido, se necessário.

curl -X POST -H "Content-Type: application/json" -d '{
    "order_id": "order-2",
    "accepted": false
}' http://localhost:8080/accept-dcc

echo ""
