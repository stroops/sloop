# 1. Overall
```text
                ┌─────────────────────┐
                │     sloop CLI       │
                │  (Cobra + Viper)    │
                └─────────┬───────────┘
                          │ gRPC / unix socket
                          ▼
                ┌─────────────────────┐
                │      sloopd         │
                │   (daemon core)     │
                └─────────┬───────────┘
                ┌───────────────────┼──────────────────────┐
                ▼                   ▼                      ▼
          Context Store      AI Router (LiteLLM)     Tool Executor
         (SQLite/LiteSQL)         │                      (git, fs, shell)
            │                     ▼                       
            ▼            Vector DB (embeddings)
          Session Memory
```
