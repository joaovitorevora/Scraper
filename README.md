# 🕷️ GeoRisk Web Scraper

> **Componente de Coleta de Dados do TCC GeoRisk**
> É responsável por monitorar portais de notícias locais, identificar crimes noticiados, geolocalizar os endereços citados e alimentar o banco de dados Firebase em tempo real.

![Badge Concluído](http://img.shields.io/static/v1?label=STATUS&message=CONCLUÍDO&color=GREEN&style=for-the-badge)
![React Native](https://img.shields.io/badge/React_Native-20232A?style=for-the-badge&logo=react&logoColor=61DAFB)
![Firebase](https://img.shields.io/badge/Firebase-039BE5?style=for-the-badge&logo=Firebase&logoColor=white)
![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)

---

## 📋 Funcionalidades

#### Este script escrito em Golang executa as seguintes tarefas automaticamente:
#### 1. Monitoramento: Acessa a página de segurança de portais de notícias.
#### 2. Extração (Scraping): Lê o HTML das notícias para encontrar textos que contenham endereços (Ruas, Bairros).
#### 3. Classificação (NLP Simples): Identifica o tipo de crime (Roubo, Furto, Tráfico, etc.) usando palavras-chave.
#### 4. Geocodificação: Converte o endereço textual em coordenadas (Latitude/Longitude) usando a API Nominatim (OpenStreetMap).
#### 5. Persistência: Salva os dados validados diretamente na coleção risk_zones do Firebase Firestore.

---

## ⚙️ Pré-requisitos

Para rodar este script, precisa ter instalado em sua máquina:
* Go (Golang): Versão 1.18 ou superior.
* Credenciais do Firebase: O arquivo de chave de serviço (KeyFirebase.json).

---

## 🔧 Instalação e Execução

## 1. Configurar Credenciais do Firebase

#### Para que o script tenha permissão de escrita no banco de dados:
#### 1. Acesse o Console do Firebase.
#### 2. Vá em Configurações do Projeto > Contas de Serviço.
#### 3. Clique em Gerar nova chave privada.
#### 4. Um arquivo .json será baixado.
#### 5. Renomeie este arquivo para KeyFirebase.json.
#### 6. Mova o arquivo para a mesma pasta onde está o scraper.go.

---

## 2.  Configurar User-Agent (Política da API)
* A API do Nominatim exige que você se identifique. Abra o arquivo main.go, procure a função geocodeAddress e altere a linha abaixo com seu email real:
```bash
req.Header.Set("User-Agent", "GeoRisk Scraper Project (seu.email@aqui.com)")
```

### 3. Rodar aplicação
```bash
cd Nome_da_pasta
go run scraper.go
```

## 🚀 O que esperar da execução:

#### 1. Confirmação de conexão com o Firebase.
#### 2. Total de links encontrados na página alvo.
#### 3. Para cada notícia, informará se houve sucesso na extração do endereço e geocodificação.
#### 4. Mensagem de sucesso ao salvar no Firestore.

#### Exemplo de Saída:
```bash
2025/11/23 10:00:00 Iniciando nova execução do scraper...
2025/11/23 10:00:02 Total de 15 links únicos encontrados.
2025/11/23 10:00:04 SUCESSO! Processando e salvando novo link: https://...
2025/11/23 10:00:05 Scraping concluído.
```

## ⚖️ Aviso Legal e Ética

* Rate Limiting: O script possui pausas programadas (time.Sleep) para respeitar os limites de requisição da API pública do Nominatim. Não remova esses delays
* Web Scraping: Este código foi desenvolvido para fins acadêmicos. O uso em larga escala deve respeitar os termos de serviço (robots.txt) dos portais de notícias alvo.
