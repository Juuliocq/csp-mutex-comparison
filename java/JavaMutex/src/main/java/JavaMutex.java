import java.util.concurrent.Executors;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.Scanner;
import java.util.List;
import java.util.ArrayList;
import java.util.concurrent.atomic.AtomicLong;

public class JavaMutex {
    private static double toSecondsDivisor = 1e9;

    private static long counter = 0;
    private static List<Long> counters = new ArrayList<>();

    private static volatile long junkValue = 0;
    private static int loopIntensity = 0;

    private static List<Double> criticalTimes = new ArrayList<>();
    private static List<Double> elapsedTimes = new ArrayList<>();
    private static List<Double> throughputs = new ArrayList<>();

    public static void main(String[] args) throws Exception {
        final int numWorkers, numIncrements, executionTimes;

        try (Scanner scanner = new Scanner(System.in)) {
            System.out.println("--- Configuração do Benchmark (Java Mutex) ---");
            System.out.print("Número de Workers/Virtual Threads: ");
            numWorkers = scanner.nextInt();
            System.out.print("Número de Incrementos por Worker: ");
            numIncrements = scanner.nextInt();
            System.out.print("Intensidade do Loop Interno (Ex: 1000): ");
            loopIntensity = scanner.nextInt();
            System.out.print("Número de rodadas: ");
            executionTimes = scanner.nextInt();
        }

        // Warm-up
        System.out.println("Aquecendo a JVM...");
        ExecutorService warmupExecutor = Executors.newVirtualThreadPerTaskExecutor();
        warmup(warmupExecutor, 100, 100);

        System.out.println("Começando");

        for (int i = 0; i < executionTimes; i++) {
            ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();
            long start = System.nanoTime();
            AtomicLong roundCriticalTime = new AtomicLong(0);

            for (int j = 0; j < numWorkers; j++) {
                executor.submit(() -> {
                    long seed = System.nanoTime();
                    for (int k = 0; k < numIncrements; k++) {
                        roundCriticalTime.addAndGet(increment(seed + k));
                    }
                });
            }

            executor.shutdown();
            executor.awaitTermination(1, TimeUnit.HOURS);

            long elapsed = System.nanoTime() - start;

            elapsedTimes.add(elapsed / toSecondsDivisor);
            criticalTimes.add(roundCriticalTime.get() / toSecondsDivisor);
            throughputs.add(((double) numWorkers * numIncrements) / (elapsed / toSecondsDivisor));
            counters.add(counter);

            counter = 0;
        }

        double totalElapsedTime = elapsedTimes.stream().mapToDouble(Double::doubleValue).sum();
        double avgElapsed = (totalElapsedTime) / executionTimes;

        double totalCriticalTime = criticalTimes.stream().mapToDouble(Double::doubleValue).sum();
        double avgCritical = (totalCriticalTime) / executionTimes;


        double totalOps = (double) numWorkers * numIncrements * executionTimes;
        double avgThroughput = totalOps / (totalElapsedTime);

        System.out.println("\n--- Resultados Java Mutex ---");
        System.out.println("Contadores: " + counters);
        System.out.println("Tempos (s): " + elapsedTimes);
        System.out.println("Tempos de seção crítica (s): " + criticalTimes);
        System.out.println("Throughputs (ops/s): " + throughputs);
        System.out.println("Junk value: " + junkValue);
        System.out.println("");
        System.out.println("Tempo médio por rodada: %.6fs\n" + avgElapsed + "s");
        System.out.println("Tempo médio de seção crítica por rodada: %.6fs\n" + avgCritical + "s");
        System.out.printf("Throughput médio: %.2f ops/s\n"), avgThroughput);
    }

    // Warm-up
    private static void warmup(ExecutorService executor, int w, int i) throws Exception {
        int original = loopIntensity;
        loopIntensity = 10;
        for (int k = 0; k < w; k++) {
            executor.submit(() -> {
                for (int l = 0; l < i; l++) increment(1);
            });
        }
        executor.shutdown();
        executor.awaitTermination(1, TimeUnit.MINUTES);
        counter = 0;
        loopIntensity = original;
    }

    // Incremento com exclusão mútua
    private static synchronized long increment(long runtimeSeed) {
        long result = runtimeSeed;
        long start = System.nanoTime();

        for (int i = 0; i < loopIntensity; i++) {
            result = result * 31 + i;
        }

        junkValue = result;
        counter++;
        return System.nanoTime() - start; // retorna tempo da seção crítica
    }
}
