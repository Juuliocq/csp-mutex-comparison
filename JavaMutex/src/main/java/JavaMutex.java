import java.lang.management.ManagementFactory;
import com.sun.management.OperatingSystemMXBean;
import java.util.concurrent.Executors;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.Scanner;
import java.util.List;
import java.util.ArrayList;
import java.util.Collections;
import java.util.concurrent.atomic.AtomicLong;

// --- Análise da Classe ---
// Esta classe recria o benchmark do Go usando o modelo de concorrência de memória compartilhada do Java.
// Em vez de canais (channels) para serializar o acesso, usamos um método 'synchronized' (increment).
// Este mecanismo garante que apenas uma thread possa executar o bloco de código sincronizado por vez,
// funcionando como uma trava (mutex) e servindo ao mesmo propósito do 'sequencer' em Go.
public class JavaMutex {

    private static final int NUM_CPU = 16;

    private static final OperatingSystemMXBean osBean =
            (OperatingSystemMXBean) ManagementFactory.getOperatingSystemMXBean();

    // Divisor para converter nanossegundos para segundos.
    private static final double NANOS_TO_SECONDS = 1e9;

    // --- Variáveis Globais (equivalentes às do Go) ---
    // 'counter' é o estado compartilhado, protegido pelo método 'synchronized'.
    private static long counter = 0;
    // 'counters' armazena o valor final do contador de cada rodada.
    // Usamos uma lista sincronizada para garantir que adições de diferentes threads (se aplicável) sejam seguras.
    private static List<Long> counters = new ArrayList<>();

    // 'junkValue' deve ser 'volatile' para garantir que as escritas feitas por uma thread
    // sejam imediatamente visíveis para outras threads, evitando que o compilador otimize o loop.
    private static volatile long junkValue = 0;

    // Listas para armazenar as métricas de cada rodada do benchmark.
    private static List<Long> criticalTimes = new ArrayList<>();
    private static List<Long> elapsedTimes = new ArrayList<>();
    private static List<Double> throughputs = new ArrayList<>();
    private static List<Double> cpuUsage = new ArrayList<>();

    // Variáveis de configuração lidas do input do usuário.
    private static int numWorkers;
    private static int loopIntensity;
    private static int numExecutions;

    public static void main(String[] args) throws InterruptedException {
        // --- Coleta de Inputs do Usuário ---
        try (Scanner scanner = new Scanner(System.in)) {
            System.out.println("--- Configuração do Benchmark (Java Mutex com Virtual Threads) ---");
            System.out.print("Número de Workers/Virtual Threads: ");
            numWorkers = scanner.nextInt();
            System.out.print("Intensidade do Loop Interno (Ex: 1000): ");
            loopIntensity = scanner.nextInt();
            System.out.print("Número de rodadas: ");
            numExecutions = scanner.nextInt();
        }

        // --- Fase de Aquecimento (Warm-up) ---
        // Essencial em Java para permitir que a JVM (Java Virtual Machine) e o compilador JIT
        // (Just-In-Time) otimizem o código antes do início das medições reais.
        System.out.println("Aquecendo a JVM...");
        warmup(100, 100);

        System.out.println("Começando o benchmark...");

        // --- Loop Principal do Benchmark ---
        for (int i = 0; i < numExecutions; i++) {
            System.gc();
            System.out.flush();

            // 'ExecutorService' gerencia o ciclo de vida das threads.
            // 'newVirtualThreadPerTaskExecutor' cria uma nova Thread Virtual para cada tarefa.
            ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();

            // Usamos AtomicLong para acumular o tempo crítico de forma segura entre as threads.
            AtomicLong roundCriticalTime = new AtomicLong(0);

            // Inicia medição de CPU
            long startCPUTime = osBean.getProcessCpuTime();

            long start = System.nanoTime(); // Use System.nanoTime() para medição de tempo.

            // Submete as tarefas dos workers para o executor.
            for (int j = 0; j < numWorkers; j++) {
                executor.submit(() -> {
                    long seed = System.nanoTime();
                    // A tarefa de cada worker chama o método sincronizado.
                    long response = increment(seed);
                    roundCriticalTime.addAndGet(response);

                });
            }

            // --- Sincronização (equivalente ao WaitGroup) ---
            // 'shutdown()' impede que novas tarefas sejam submetidas.
            executor.shutdown();
            // 'awaitTermination()' bloqueia a thread principal até que todas as tarefas
            // no executor tenham sido concluídas. É o equivalente ao 'wg.Wait()' em Go.
            executor.awaitTermination(1, TimeUnit.HOURS);

            long elapsed = System.nanoTime() - start;

            long endCPUTime = osBean.getProcessCpuTime();

            // --- Coleta de Métricas da Rodada ---
            // Adiciona os resultados da rodada às listas de métricas.
            counters.add(counter);
            criticalTimes.add(roundCriticalTime.get());

            elapsedTimes.add(elapsed);
            double elapsedSeconds = elapsed / NANOS_TO_SECONDS;

            double throughput = ((double) numWorkers) / elapsedSeconds;
            throughputs.add(throughput);

            double cpuTimeSeconds = (endCPUTime - startCPUTime) / NANOS_TO_SECONDS;
            double cpuPercent = (cpuTimeSeconds / elapsedSeconds) * 100;
            cpuUsage.add(cpuPercent);

            // Reseta o contador para a próxima rodada, garantindo medições independentes.
            counter = 0;
        }

        // --- Cálculo e Exibição dos Resultados Finais ---
        double totalElapsed = elapsedTimes.stream().mapToDouble(Long::longValue).sum();
        double avgElapsed = elapsedTimes.stream().mapToDouble(Long::longValue).average().orElse(0);

        double avgCritical = criticalTimes.stream().mapToDouble(Long::longValue).average().orElse(0);

        double totalOps = (double) numWorkers * numExecutions;
        double avgThroughput = totalOps / (totalElapsed / NANOS_TO_SECONDS);

        double avgCpuUsage = cpuUsage.stream().mapToDouble(Double::doubleValue).average().orElse(0);
        double finalCpuUsage = (avgCpuUsage / NUM_CPU);

        boolean raceConditionDetected = counters.stream().anyMatch(counter -> counter != numWorkers);

        System.out.println("\n--- Resultados Finais (Java Mutex) ---");
        System.out.println("Valores finais do contador por rodada: " + counters);
        System.out.printf("Tempos totais por rodada (s): %s\n", elapsedTimes);
        System.out.printf("Tempos de seção crítica por rodada (s): %s\n", criticalTimes);
        System.out.printf("Throughputs por rodada (ops/s): %s\n", throughputs);
        System.out.printf("Uso de CPU por rodada (%%): %s\n", cpuUsage);
        System.out.println("Valor final de Junk: " + junkValue);
        System.out.println("\n--- Médias ---");
        System.out.printf("Houve race condition?: %s\n", raceConditionDetected ? "SIM" : "NÃO");
        System.out.printf("Tempo médio global: %.8fs\n", avgElapsed / NANOS_TO_SECONDS);
        System.out.printf("Tempo médio de seção crítica global: %.8fs\n", avgCritical / NANOS_TO_SECONDS);
        System.out.printf("Throughput médio global: %.2f ops/s\n", avgThroughput);
        System.out.printf("Uso de CPU médio global: %.4f %%\n", finalCpuUsage);
    }

    // O método de warm-up prepara a JVM executando uma carga de trabalho semelhante, mas menor.
    private static void warmup(int w, int i) throws InterruptedException {
        int originalIntensity = loopIntensity;
        loopIntensity = 10;
        ExecutorService warmupExecutor = Executors.newVirtualThreadPerTaskExecutor();

        for (int k = 0; k < w; k++) {
            warmupExecutor.submit(() -> {
                for (int l = 0; l < i; l++) {
                    increment(1);
                }
            });
        }
        warmupExecutor.shutdown();
        warmupExecutor.awaitTermination(1, TimeUnit.MINUTES);

        // Reseta o estado após o aquecimento.
        counter = 0;
        junkValue = 0;
        loopIntensity = originalIntensity;
    }

    // --- Seção Crítica ---
    // A palavra-chave 'synchronized' garante que apenas uma thread possa executar este método
    // por vez (exclusão mútua). Este é o mecanismo de "lock" (trava) que protege o acesso
    // concorrente ao 'counter' e 'junkValue'.
    private static synchronized long increment(long runtimeSeed) {
        long start = System.nanoTime();

        // Simula o processamento de CPU (a "carga de trabalho").
        long result = runtimeSeed;
        for (int i = 0; i < loopIntensity; i++) {
            result = result * 31 + i;
        }

        // Modificação segura do estado global.
        junkValue = result;
        counter++;
        return System.nanoTime() - start; // Retorna a duração da seção crítica.
    }
}