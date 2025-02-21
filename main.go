package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// Order representa um pedido recebido na aplicação de pedidos
type Order struct {
	OrderID         string  `json:"order_id"`
	CustomerID      string  `json:"customer_id"`
	Amount          float64 `json:"amount"`   // Valor original (usado na busca do workflow)
	Currency        string  `json:"currency"` // Moeda original (usada na busca do workflow)
	CardBrand       string  `json:"card_brand"`
	DCCAccepted     bool    `json:"dcc_accepted"`
	PaymentAmount   float64 `json:"payment_amount"`   // Valor a ser processado
	PaymentCurrency string  `json:"payment_currency"` // Moeda a ser utilizada no processamento
}

// WorkflowStep representa um passo do workflow
type WorkflowStep struct {
	StepID  string `json:"step_id"`
	Service string `json:"service"`
	Status  string `json:"status"`
}

// Workflow representa um workflow de PBC armazenado no banco
type Workflow struct {
	WorkflowID string            `json:"workflow_id"`
	Criteria   map[string]string `json:"criteria"`
	Steps      []WorkflowStep    `json:"steps"`
}

// Simulação de um banco de dados em memória para workflows
var workflowsDB = []Workflow{
	{
		WorkflowID: "wf-payment-dcc-proposal",
		Criteria: map[string]string{
			"payment_type": "credit_card",
			"currency":     "USD",
		},
		Steps: []WorkflowStep{
			{"dcc_proposal", "PBC_DCC", "pending"},
			{"payment_processing", "PBC_Payment", "pending"},
		},
	},
	{
		WorkflowID: "wf-payment-generic",
		Criteria: map[string]string{
			"payment_type": "credit_card",
			"currency":     "BRL",
		},
		Steps: []WorkflowStep{
			{"payment_processing", "PBC_Payment", "pending"},
		},
	},
}

// Mutex para acesso seguro ao banco de pedidos
var orderMutex sync.Mutex
var ordersDB = make(map[string]Order)

// WorkflowInstance guarda o workflow selecionado e o índice do passo atual para um pedido
type WorkflowInstance struct {
	Workflow  *Workflow
	StepIndex int
}

// Mapeia OrderID para a instância do workflow iniciado
var workflowInstances = make(map[string]*WorkflowInstance)

// createOrder recebe um pedido e inicia o workflow correspondente
func createOrder(w http.ResponseWriter, r *http.Request) {
	var order Order
	err := json.NewDecoder(r.Body).Decode(&order)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	orderMutex.Lock()
	order.OrderID = fmt.Sprintf("order-%d", len(ordersDB)+1)
	// Inicializa os campos de pagamento com os valores originais
	order.PaymentAmount = order.Amount
	order.PaymentCurrency = order.Currency
	ordersDB[order.OrderID] = order
	orderMutex.Unlock()

	log.Printf("[Pedidos] Pedido criado: %+v", order)

	// Seleciona o workflow com base nos campos originais do pedido
	workflow := findWorkflow(order)
	if workflow == nil {
		log.Printf("[Orquestrador] Nenhum workflow encontrado para OrderID: %s", order.OrderID)
		return
	}

	// Armazena a instância do workflow para este pedido
	workflowInstances[order.OrderID] = &WorkflowInstance{
		Workflow:  workflow,
		StepIndex: 0,
	}

	// Inicia a execução dos passos
	go executeWorkflowSteps(order, workflowInstances[order.OrderID])

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

// findWorkflow busca o workflow adequado para um pedido com base nos campos originais
func findWorkflow(order Order) *Workflow {
	// Utiliza o campo original 'Currency' para selecionar o workflow
	for _, wf := range workflowsDB {
		if wf.Criteria["payment_type"] == "credit_card" && wf.Criteria["currency"] == order.Currency {
			return &wf
		}
	}
	return nil
}

// executeWorkflowSteps executa os passos do workflow a partir do índice atual armazenado na instância
func executeWorkflowSteps(order Order, instance *WorkflowInstance) {
	for i := instance.StepIndex; i < len(instance.Workflow.Steps); i++ {
		step := instance.Workflow.Steps[i]
		log.Printf("[Orquestrador] Executando passo %s com %s para OrderID: %s", step.StepID, step.Service, order.OrderID)

		if step.StepID == "dcc_proposal" {
			// Envia a proposta de DCC e pausa o workflow aguardando resposta do cliente
			proposeDCC(order)
			instance.StepIndex = i + 1
			return
		}

		if step.Service == "PBC_Payment" {
			// Processa o pagamento utilizando os valores atualizados para pagamento
			processPayment(order)
		}
	}

	log.Printf("[Orquestrador] Workflow concluído para OrderID: %s", order.OrderID)
	delete(workflowInstances, order.OrderID)
}

// proposeDCC envia a proposta de conversão de moeda (DCC) ao cliente
func proposeDCC(order Order) {
	convertedAmount := order.PaymentAmount * 0.85
	log.Printf("[PBC_DCC] Ofertando conversão de moeda para OrderID: %s. Proposta: converter %f %s para %f BRL",
		order.OrderID, order.PaymentAmount, order.PaymentCurrency, convertedAmount)
}

// processPayment simula o processamento do pagamento utilizando os campos de pagamento
func processPayment(order Order) {
	log.Printf("[PBC_Payment] Processando pagamento para OrderID: %s, valor: %.2f %s", order.OrderID, order.PaymentAmount, order.PaymentCurrency)
}

// acceptDCCHandler processa a resposta do cliente à oferta de DCC e retoma o workflow
func acceptDCCHandler(w http.ResponseWriter, r *http.Request) {
	var response struct {
		OrderID     string  `json:"order_id"`
		Accepted    bool    `json:"accepted"`
		NewAmount   float64 `json:"new_amount,omitempty"`
		NewCurrency string  `json:"new_currency,omitempty"`
	}
	err := json.NewDecoder(r.Body).Decode(&response)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	orderMutex.Lock()
	order, exists := ordersDB[response.OrderID]
	if exists {
		if response.Accepted {
			log.Printf("[PBC_DCC] Cliente aceitou DCC para OrderID: %s. Novo valor: %f %s",
				response.OrderID, response.NewAmount, response.NewCurrency)
			// Atualiza somente os campos de pagamento, mantendo os originais para busca
			order.PaymentAmount = response.NewAmount
			order.PaymentCurrency = response.NewCurrency
			order.DCCAccepted = true
		} else {
			log.Printf("[PBC_DCC] Cliente recusou DCC para OrderID: %s. Mantendo valor original: %.2f %s",
				response.OrderID, order.PaymentAmount, order.PaymentCurrency)
			order.DCCAccepted = false
		}
		ordersDB[response.OrderID] = order
	}
	orderMutex.Unlock()

	// Retoma o workflow utilizando a instância já armazenada, garantindo que o passo de pagamento seja executado
	if instance, ok := workflowInstances[response.OrderID]; ok {
		log.Printf("[Orquestrador] Retomando workflow para OrderID: %s", response.OrderID)
		go executeWorkflowSteps(order, instance)
	} else {
		log.Printf("[Orquestrador] Nenhuma instância de workflow encontrada para OrderID: %s", response.OrderID)
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/create-order", createOrder)
	http.HandleFunc("/accept-dcc", acceptDCCHandler)

	log.Println("[API] Servidor iniciado na porta 8080")
	http.ListenAndServe(":8080", nil)
}
