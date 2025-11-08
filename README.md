# Análise de Overhead em Sincronização: Go Channels vs. Java Locks

Este repositório contém as implementações e os resultados do Trabalho de Conclusão de Curso (TCC) que investigou o **overhead de sincronização** entre dois paradigmas de concorrência modernos:

1.  **Passagem de Mensagens (CSP):** Implementado com **Channels síncronos em Go** (Goroutines).
2.  **Exclusão Mútua (Locks):** Implementado com **Locks Pessimistas (`synchronized`) em Java** (Virtual Threads do Project Loom).

## Objetivo

O estudo buscou comparar o impacto no desempenho, escalabilidade e uso de recursos ao serializar o acesso a um recurso crítico em condições de alta concorrência.

## Metodologia

Um benchmark de **Acesso a Recurso Crítico Compartilhado** (contador em memória) foi executado, analisando dois cenários principais:
1.  **Escalabilidade de Workers:** Aumentando a **contenção** (o número de unidades concorrentes).
2.  **Intensidade da Carga Crítica:** Aumentando o **trabalho tem computacional** dentro da seção crítica.

## Resultados e Conclusões Chave

A análise confirmou que dentro dos cenários analisados, **não existe um modelo superior**, mas sim contextos específicos de vantagem:

| Cenário | Comportamento Vencedor | Conclusão |
| :--- | :--- | :--- |
| **Alta Contenção** (>= 16384 Workers) | **Java (`synchronized` / Virtual Threads)** | Apresentou melhor escalabilidade e maior *throughput* sob contenção intensa. Este desempenho, porém, veio acompanhado de maior **uso de CPU** devido a uma política de escalonamento mais agressiva. |
| **Baixa/Média Concorrência** (<= 8192 mil Workers) | **Go (Channels)** | Demonstrou maior **eficiência** e **economia de recursos** (CPU). O baixo custo do *rendezvous* dos canais em baixa contenção proporcionou menores tempos de conclusão. |
| **Carga Crítica Baixa/Média** (<= 8129 iterações ) | **Java (`synchronized` / Virtual Threads)** | Mostrou vantagem. Em tarefas menos pesadas , o *overhead* de sincronização do Go ainda era o fator limitante. |
| **Carga Crítica Pesada** (>= 16384 iterações ) | **Go (Channels)** | Obteve uma leve vantagem. Em tarefas mais pesadas, o *overhead* de sincronização do Go tornou-se relativamente menor em relação ao tempo de cálculo. |

**Conclusão Geral:** O modelo de **exclusão mútua com threads virtuais** (Java) mostrou-se mais robusto para um grande volume de tarefas e alta contenção, embora possa introduzir problemas clássicos de concorrência, enquanto o paradigma de **passagem de mensagens** (Go) destacou-se pela eficiência e economia de recursos em menor escala, assim como a capacidade de resolver problemas de concorrência por design.

## Estrutura do Repositório

| Pasta | Conteúdo |
| :--- | :--- |
| `go/` | Implementação do benchmark com Goroutines e Canais síncronos. |
| `java/` | Implementação do benchmark com Virtual Threads e `synchronized`. |
| `escalabilidade-workers.xlsx` | Contém os resultados e gráficos do cenário de Escalabilidade de Workers. |
| `intensidade-loop.xlsx` | Contém os resultados e gráficos do cenário de Intensidade De Carga Crítica. |

---