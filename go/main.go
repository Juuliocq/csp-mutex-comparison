package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// --- Variáveis Globais ---
var (
	// Constante auxiliar para cálculos de uso de CPU
	NUM_CPU = 16

	// counter é o estado compartilhado que será modificado de forma concorrente.
	// O objetivo do padrão neste código é garantir que o incremento de 'counter'
	// seja feito de forma segura (sem race conditions).
	counter int64 = 0

	// counters armazena o valor final de 'counter' de cada rodada do benchmark.
	counters []int64

	// junkValue é usado para garantir que o compilador não otimize
	// o loop de processamento dentro da seção crítica, assegurando que o
	// trabalho de CPU simulado realmente aconteça.
	junkValue int64 = 0

	// currentCriticalTime acumula o tempo gasto dentro da seção crítica em uma única rodada.
	currentCriticalTime time.Duration

	// criticalTimes armazena a duração total da seção crítica para cada rodada.
	criticalTimes []time.Duration

	// elapsedTimes armazena o tempo total de execução de cada rodada.
	elapsedTimes []time.Duration

	// throughputs armazena a vazão (operações por segundo) de cada rodada.
	throughputs []float64

	// cpuUsage armazena o uso total de cpu de cada rodada.
	cpuUsage []float64

	// --- Variáveis de Configuração ---
	numWorkers     int // Quantidade de goroutines "trabalhadoras".
	numIncrements  int // Quantas tarefas cada worker irá gerar.
	executionTimes int // Quantas vezes o benchmark completo será executado.
	loopIntensity  int // Controla a carga de trabalho de CPU na seção crítica.
)

// sequencer é a goroutine central que gerencia o acesso ao estado compartilhado.
// Apenas *esta* goroutine tem permissão para modificar 'counter' e 'junkValue'.
// 'requests' é um canal "receive-only" (<-chan) para esta função.
func sequencer(wg *sync.WaitGroup, requests <-chan int64) {
	// Garante que wg.Done() seja chamado quando a função terminar,
	// sinalizando ao WaitGroup que esta goroutine completou sua execução.
	defer wg.Done()

	// O loop 'for range' em um canal continuará a receber valores
	// até que o canal seja fechado.
	for requestSeed := range requests {
		// --- INÍCIO DA SEÇÃO CRÍTICA ---
		// O código a seguir é executado serialmente para cada mensagem recebida.
		start := time.Now()

		// Simula um processamento de CPU intensivo.
		// Esta é a "carga de trabalho" que precisa ser protegida.
		result := requestSeed
		for i := 0; i < loopIntensity; i++ {
			result = result*31 + int64(i)
		}

		// Modificação segura do estado global. Como apenas o sequencer
		// executa este código, não há risco de condição de corrida aqui.
		junkValue = result
		counter++

		// Acumula o tempo gasto dentro da seção crítica.
		currentCriticalTime += time.Since(start)
		// --- FIM DA SEÇÃO CRÍTICA ---
	}
}

// worker representa uma goroutine "produtora".
// Ela gera trabalho e o envia para o sequencer através do canal.
// 'wg' é usado para sinalizar que o worker concluiu seu trabalho.
// 'requests' é um canal "send-only" (chan<-) para esta função.
func worker(wg *sync.WaitGroup, requests chan<- int64, seed int64) {
	// Garante que wg.Done() seja chamado quando a função terminar,
	// sinalizando ao WaitGroup que esta goroutine completou sua execução.
	defer wg.Done()

	// Cada worker envia 'numIncrements' mensagens para o canal.
	for k := 0; k < numIncrements; k++ {
		// Envia um valor para o canal 'requests'.
		// A execução deste worker irá *bloquear* nesta linha se o canal
		// estiver cheio (neste caso, se o sequencer não estiver pronto para receber).
		// Isso naturalmente limita a velocidade dos workers à capacidade de
		// processamento do sequencer.
		requests <- seed + int64(k)
	}
}

func main() {
	runtime.GOMAXPROCS(NUM_CPU)
	// --- Coleta de Inputs do Usuário ---
	fmt.Println("--- Configuração do Benchmark (Go Channels c/ Processamento Crítico) ---")
	fmt.Print("Número de Workers/Goroutines: ")
	fmt.Scanln(&numWorkers)
	fmt.Print("Número de Incrementos por Worker: ")
	fmt.Scanln(&numIncrements)
	fmt.Print("Intensidade do Loop Interno (Ex: 1000): ")
	fmt.Scanln(&loopIntensity)
	fmt.Print("Número de rodadas: ")
	fmt.Scanln(&executionTimes)

	// --- Fase de Aquecimento (Warm-up) ---
	// Essencial em benchmarks para permitir que o runtime do Go
	// otimize o código. As primeiras execuções podem ser mais lentas,
	// então elas são descartadas para não poluir os resultados reais.
	fmt.Println("Aquecendo a Go Runtime...")
	warmup(100, 100) // Executa uma carga de trabalho menor para o aquecimento.

	fmt.Println("Começando o benchmark...")

	// --- Loop Principal do Benchmark ---
	for i := 0; i < executionTimes; i++ {

		// Reseta o contador e o tempo em seção crítica atual
		counter = 0
		currentCriticalTime = 0

		// Cria um novo objeto "Process" representando o processo atual
		// `os.Getpid()` retorna o PID (Process ID) do programa Go em execução
		proc, _ := process.NewProcess(int32(os.Getpid()))

		// Captura os tempos de CPU do processo nesse instante (início da medição).
		cpuStart, _ := proc.Times()
		start := time.Now()

		// Cria um novo canal para a comunicação entre workers e o sequencer.
		// O canal é "unbuffered" (sem buffer), o que significa que o envio (worker)
		// e o recebimento (sequencer) devem ocorrer simultaneamente.
		requests := make(chan int64)

		// WaitGroup do sequencer é usado para esperar que a goroutine sequencer termine
		var sequencerWg sync.WaitGroup
		sequencerWg.Add(1)

		// Inicia a goroutine do sequencer, que começará a esperar por mensagens.
		go sequencer(&sequencerWg, requests)

		// WaitGroup de workers é usado para esperar que todos os workers terminem.
		var workersWg sync.WaitGroup
		workersWg.Add(numWorkers) // Define o número de goroutines que o WaitGroup deve esperar.

		// Inicia as goroutines dos workers.
		for j := 0; j < numWorkers; j++ {
			// A semente (seed) é baseada no tempo para variar os valores enviados.
			go worker(&workersWg, requests, time.Now().UnixNano())
		}

		// A execução da goroutine 'main' pausa aqui até que todos os workers
		// chamem 'workersWg.Done()'.
		workersWg.Wait()

		// Após todos os workers terminarem de enviar, o canal 'requests' é fechado.
		// Fechar o canal é o sinal para o 'for range' no sequencer de que não
		// haverá mais mensagens, permitindo que ele termine sua execução graciosamente.
		close(requests)

		// A execução da goroutine 'main' pausa aqui até que o sequencer chame 'sequencerWg.Done()'.
		sequencerWg.Wait()

		elapsed := time.Since(start)

		// Captura novamente os tempos de CPU do processo depois da execução.
		cpuEnd, _ := proc.Times()

		// Calcula o tempo total de CPU gasto entre as duas medições.
		// Subtrai o tempo de CPU anterior (cpuStart) do tempo atual (cpuEnd) — separadamente para modo usuário e modo sistema.
		// O resultado (em segundos) representa quanto tempo efetivo de CPU foi consumido nesse intervalo.
		cpuUsed := (cpuEnd.User - cpuStart.User) + (cpuEnd.System - cpuStart.System)

		// Calcula o tempo de CPU usado em porcentagem de uso relativo ao tempo real decorrido.
		cpuPercent := (cpuUsed / elapsed.Seconds()) * 100

		counters = append(counters, counter)
		elapsedTimes = append(elapsedTimes, elapsed)
		criticalTimes = append(criticalTimes, currentCriticalTime)
		throughput := float64(numWorkers*numIncrements) / elapsed.Seconds()
		throughputs = append(throughputs, throughput)
		cpuUsage = append(cpuUsage, cpuPercent)
	}

	// --- Cálculo e Exibição dos Resultados Finais ---
	totalElapsedTimes := 0.0
	for _, et := range elapsedTimes {
		totalElapsedTimes += et.Seconds()
	}

	averageElapsedTime := totalElapsedTimes / float64(executionTimes)
	totalOps := float64(numWorkers * numIncrements * executionTimes)
	averageThroughput := totalOps / totalElapsedTimes

	totalCriticalTime := 0.0
	for _, ct := range criticalTimes {
		totalCriticalTime += ct.Seconds()
	}
	averageCriticalTime := totalCriticalTime / float64(executionTimes)

	totalCpuUsage := 0.0
	for _, cpu := range cpuUsage {
		totalCpuUsage += cpu
	}

	// Calcula a média de uso de CPU em porcentagem considerando todos os núcleos.
	// Divide o tempo médio de CPU usado por execução pelo número de núcleos e multiplica por 100.
	averageCpuUse := ((totalCpuUsage / float64(executionTimes)) / float64(runtime.NumCPU()))

	fmt.Println("\n--- Resultados Finais (Go Channels) ---")
	fmt.Println("Valores finais do contador por rodada:", counters)
	fmt.Printf("Tempos totais por rodada (s): %v\n", durationsToSeconds(elapsedTimes))
	fmt.Printf("Tempos de seção crítica por rodada (s): %v\n", durationsToSeconds(criticalTimes))
	fmt.Printf("Throughputs por rodada (ops/s): %v\n", throughputs)
	fmt.Printf("Uso de CPU por rodada (%%): %v\n", cpuUsage)
	fmt.Println("Valor final de Junk:", junkValue)
	fmt.Printf("\n--- Médias ---\n")
	fmt.Printf("Tempo médio global: %.4fs\n", averageElapsedTime)
	fmt.Printf("Tempo médio de seção crítica global: %.4fs\n", averageCriticalTime)
	fmt.Printf("Throughput médio global: %.2f ops/s\n", averageThroughput)
	fmt.Printf("Uso de CPU médio global: %.4f %%\n", averageCpuUse)
}

// warmup executa uma versão menor do benchmark para preparar o ambiente de execução.
func warmup(w int, i int) {
	originalIntensity := loopIntensity
	loopIntensity = 10                                   // Usa uma intensidade baixa para não demorar muito.
	defer func() { loopIntensity = originalIntensity }() // Restaura o valor original ao final.

	requests := make(chan int64)
	var workersWg sync.WaitGroup

	var sequencerWg sync.WaitGroup
	sequencerWg.Add(1)

	go sequencer(&sequencerWg, requests)

	workersWg.Add(w)
	for range w {
		go func() {
			defer workersWg.Done()
			for range i {
				requests <- int64(1)
			}
		}()
	}

	workersWg.Wait()
	close(requests)

	sequencerWg.Wait()

	// Reseta o estado global após o aquecimento para não interferir
	// com as medições reais do benchmark.
	counter = 0
	junkValue = 0
	currentCriticalTime = 0
}

// durationsToSeconds é uma função utilitária para formatar a saída.
func durationsToSeconds(durations []time.Duration) []float64 {
	result := make([]float64, len(durations))
	for i, d := range durations {
		result[i] = d.Seconds()
	}
	return result
}
