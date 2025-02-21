#!/bin/bash
# Cria um pedido via endpoint /create-order
# Salve este arquivo como create_order.sh e torne-o executável com:
# chmod +x create_order.sh

curl -X POST -H "Content-Type: application/json" -d '{
    "customer_id": "cliente456",
    "amount": 100.00,
    "currency": "USD",
    "card_brand": "VISA"
}' http://localhost:8080/create-order

echo ""
