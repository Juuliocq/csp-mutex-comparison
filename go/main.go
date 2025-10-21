package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// --- Variáveis Globais de Benchmark ---
var (
	// NUM_CPU define o número de threads do S.O. que podem executar código
	// simultaneamente. É usado para configurar GOMAXPROCS.
	NUM_CPU = 16

	// counter é o estado compartilhado que será incrementado de forma concorrente.
	// O padrão Sequencer garante que este incremento seja seguro (sem race conditions).
	counter int64 = 0

	// counters armazena o valor final de 'counter' de cada rodada do benchmark,
	// usado para verificar se ocorreram race conditions (se algum valor != numWorkers).
	counters []int64

	// junkValue é usado para garantir que o compilador não otimize e remova
	// o loop de processamento, assegurando que o trabalho de CPU simulado aconteça.
	junkValue int64 = 0

	// currentCriticalTime acumula o tempo gasto dentro da seção crítica em uma única rodada.
	currentCriticalTime time.Duration

	// --- Variáveis de Coleta de Métricas ---

	// criticalTimes armazena a duração total da seção crítica para cada rodada.
	criticalTimes []time.Duration

	// elapsedTimes armazena o tempo total de execução de cada rodada.
	elapsedTimes []time.Duration

	// throughputs armazena a vazão (operações por segundo) de cada rodada.
	throughputs []float64

	// cpuUsage armazena o uso de CPU (em porcentagem, pode exceder 100%) de cada rodada.
	cpuUsage []float64

	// --- Variáveis de Configuração do Teste ---
	numWorkers     int // Quantidade de goroutines "trabalhadoras".
	executionTimes int // Quantas vezes o benchmark completo será executado (rodadas).
	loopIntensity  int // Controla a carga de trabalho de CPU na seção crítica.
)

// sequencer é a goroutine central que serializa o acesso ao estado compartilhado.
// Apenas esta goroutine tem permissão para modificar counter.
// 'request é um canal receive-only (<-chan) para esta função.
func sequencer(wg *sync.WaitGroup, requests <-chan int64) {
	// Garante que wg.Done() seja chamado ao final, sinalizando que a goroutine terminou.
	defer wg.Done()

	// O loop 'for range' em um canal continuará a receber valores
	// até que o canal seja fechado e todos os valores tenham sido processados.
	for requestSeed := range requests {
		// --- INÍCIO DA SEÇÃO CRÍTICA ---
		// O código a seguir é a seção crítica: é executado serialmente
		// para cada mensagem recebida, garantindo acesso exclusivo ao estado.
		start := time.Now()

		// Simula um processamento de CPU intensivo.
		// Esta é a "carga de trabalho" que precisa ser protegida pela sincronização.
		result := requestSeed
		for i := 0; i < loopIntensity; i++ {
			result = result*31 + int64(i)
		}

		// Modificação segura do estado global. Como apenas o sequencer
		// executa este código, não há risco de condição de corrida.
		junkValue = result
		counter++

		// Acumula o tempo gasto dentro da seção crítica nesta rodada.
		currentCriticalTime += time.Since(start)
		// --- FIM DA SEÇÃO CRÍTICA ---
	}
}

// worker representa uma goroutine "produtora".
// Ela gera uma única unidade de trabalho e a envia para o sequencer.
// wg sinaliza que o worker concluiu sua tarefa.
// requests é um canal send-only (chan<-) para esta função.
func worker(wg *sync.WaitGroup, requests chan<- int64, seed int64) {
	// Garante que wg.Done() seja chamado ao final, sinalizando que a goroutine terminou.
	defer wg.Done()

	// Envia um valor para o canal 'requests'. A execução deste worker
	// irá *bloquear* nesta linha se o canal estiver cheio (neste caso, se o
	// sequencer não estiver pronto para receber). Isso naturalmente limita a
	// velocidade dos workers à capacidade de processamento do sequencer.
	requests <- seed
}

func main() {
	// Define o número máximo de threads do S.O. a serem usadas pelo programa Go.
	runtime.GOMAXPROCS(NUM_CPU)

	// --- Coleta de Inputs do Usuário ---
	fmt.Println("--- Configuração do Benchmark (Go Channels) ---")
	fmt.Print("Número de Workers/Goroutines: ")
	fmt.Scanln(&numWorkers)
	fmt.Print("Intensidade do Loop (carga de trabalho): ")
	fmt.Scanln(&loopIntensity)
	fmt.Print("Número de rodadas de execução: ")
	fmt.Scanln(&executionTimes)

	// --- Fase de Aquecimento (Warm-up) ---
	// Essencial em benchmarks para permitir que o runtime
	// se estabilize. As primeiras execuções são descartadas para não poluir os resultados.
	fmt.Println("Aquecendo a Go Runtime...")
	warmup()
	fmt.Println("Começando o benchmark...")

	// --- Loop Principal do Benchmark ---
	for i := 0; i < executionTimes; i++ {
		// Força a execução do Garbage Collector antes de cada rodada para
		// minimizar sua interferência nas medições de tempo.
		runtime.GC()
		debug.FreeOSMemory()

		// Obtém o processo atual para medição de uso de CPU.
		proc, _ := process.NewProcess(int32(os.Getpid()))

		// Captura os tempos de CPU (User e System) no início da rodada.
		cpuStart, _ := proc.Times()
		start := time.Now() // Inicia a medição do tempo de parede (wall-clock).

		// Cria um canal sem buffer. O envio e o recebimento devem ser síncronos.
		requests := make(chan int64)

		// WaitGroup para esperar a finalização do sequencer.
		var sequencerWg sync.WaitGroup
		sequencerWg.Add(1)
		go sequencer(&sequencerWg, requests)

		// WaitGroup para esperar a finalização de todos os workers.
		var workersWg sync.WaitGroup
		workersWg.Add(numWorkers)

		// Inicia as goroutines dos workers.
		for j := 0; j < numWorkers; j++ {
			go worker(&workersWg, requests, time.Now().UnixNano())
		}

		// Aguarda até que todos os workers tenham enviado suas requisições.
		workersWg.Wait()

		// Fecha o canal. Isso sinaliza ao 'for range' do sequencer que não
		// haverá mais mensagens, permitindo que ele termine sua execução.
		close(requests)

		// Aguarda o sequencer terminar de processar todas as mensagens restantes.
		sequencerWg.Wait()

		// --- Coleta de Métricas da Rodada ---
		elapsed := time.Since(start) // Tempo total da rodada.

		// Captura os tempos de CPU no final da rodada.
		cpuEnd, _ := proc.Times()

		// Calcula o tempo de CPU (em segundos) efetivamente consumido pelo processo.
		cpuUsed := (cpuEnd.User - cpuStart.User) + (cpuEnd.System - cpuStart.System)

		// Calcula o uso de CPU como uma porcentagem do tempo decorrido.
		// Um valor de 250% significa que o processo, em média, utilizou 2.5 núcleos de CPU.
		cpuPercent := (cpuUsed / elapsed.Seconds()) * 100

		// Armazena as métricas desta rodada.
		counters = append(counters, counter)
		elapsedTimes = append(elapsedTimes, elapsed)
		criticalTimes = append(criticalTimes, currentCriticalTime)
		throughput := float64(numWorkers) / elapsed.Seconds()
		throughputs = append(throughputs, throughput)
		cpuUsage = append(cpuUsage, cpuPercent)

		// Reseta os contadores para a próxima rodada.
		counter = 0
		currentCriticalTime = 0
	}

	// --- Cálculo e Exibição dos Resultados Finais ---
	totalElapsedTimes := 0.0
	for _, et := range elapsedTimes {
		totalElapsedTimes += et.Seconds()
	}
	averageElapsedTime := totalElapsedTimes / float64(executionTimes)

	totalOps := float64(numWorkers * executionTimes)
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
	averageCpuPercent := totalCpuUsage / float64(executionTimes)

	// Calcula a utilização média normalizada de CPU.
	// Pega a média de uso de CPU (que pode ser >100%) e divide pelo número de
	// núcleos disponíveis, resultando na porcentagem de utilização total da máquina.
	averageCpuNormalized := averageCpuPercent / float64(runtime.NumCPU())

	// Verifica se o contador final de alguma rodada foi diferente do esperado.
	raceConditionOccurred := false
	for _, n := range counters {
		if n != int64(numWorkers) {
			raceConditionOccurred = true
			break
		}
	}

	fmt.Println("\n--- Resultados Finais (Go Channels) ---")
	fmt.Println("Valores finais do contador por rodada:", counters)
	fmt.Printf("Tempos totais por rodada (s): %v\n", durationsToSeconds(elapsedTimes))
	fmt.Printf("Tempos de seção crítica por rodada (s): %v\n", durationsToSeconds(criticalTimes))
	fmt.Printf("Throughputs por rodada (ops/s): %v\n", throughputs)
	fmt.Printf("Uso de CPU por rodada (%% de 1 núcleo): %v\n", cpuUsage)
	fmt.Println("Valor final de Junk:", junkValue)
	fmt.Printf("\n--- Médias ---\n")
	fmt.Printf("Houve race condition?: %t\n", raceConditionOccurred)
	fmt.Printf("Tempo médio por rodada: %.8fs\n", averageElapsedTime)
	fmt.Printf("Tempo médio de seção crítica por rodada: %.8fs\n", averageCriticalTime)
	fmt.Printf("Throughput médio global: %.2f ops/s\n", averageThroughput)
	fmt.Printf("Uso de CPU médio (relativo a 1 núcleo): %.2f %%\n", averageCpuPercent)
	fmt.Printf("Utilização de CPU média (normalizada para todos os núcleos): %.2f %%\n", averageCpuNormalized)
}

// warmup executa uma versão menor e mais leve do benchmark para preparar o ambiente.
func warmup() {
	// Salva as configurações originais para restaurá-las depois.
	originalIntensity := loopIntensity
	originalNumWorkers := numWorkers

	// Define valores menores para um aquecimento rápido.
	loopIntensity = 10
	numWorkers = 100

	// Garante que as configurações originais sejam restauradas ao final da função.
	defer func() {
		loopIntensity = originalIntensity
		numWorkers = originalNumWorkers
	}()

	requests := make(chan int64)
	var workersWg, sequencerWg sync.WaitGroup

	sequencerWg.Add(1)
	go sequencer(&sequencerWg, requests)

	workersWg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(&workersWg, requests, time.Now().UnixNano())
	}

	workersWg.Wait()
	close(requests)
	sequencerWg.Wait()

	// Reseta o estado global para não interferir nas medições reais.
	counter = 0
	junkValue = 0
	currentCriticalTime = 0
}

// durationsToSeconds é uma função utilitária para converter um slice de
// time.Duration para um slice de float64 (segundos) para exibição.
func durationsToSeconds(durations []time.Duration) []float64 {
	result := make([]float64, len(durations))
	for i, d := range durations {
		result[i] = d.Seconds()
	}
	return result
}
