package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	firebase "firebase.google.com/go"
	"github.com/PuerkitoBio/goquery"
	"google.golang.org/api/option"
	latlng "google.golang.org/genproto/googleapis/type/latlng"
)

// Estrutura para guardar os resultados da geocodificação do Nominatim
type GeocodeResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// Mapa de crimes e palavras-chave
var crimeKeywords = map[string][]string{
	"furto":     {"furto", "furtado", "furtaram", "furtada"},
	"roubo":     {"roubo", "roubado", "roubada", "roubaram", "assalto"},
	"homicidio": {"homicídio", "assassinato", "morto", "assassinado", "assassinada"},
	"trafico":   {"tráfico", "drogas", "entorpecente"},
	"agressao":  {"agressão", "espancamento", "violência física"},
	"sequestro": {"sequestro", "sequestrado", "sequestrada"},
}

// detectCrimeType identifica o tipo de crime com base em palavras-chave no texto.
func detectCrimeType(text string) string {
	lowerText := strings.ToLower(text)
	for crimeType, keywords := range crimeKeywords {
		for _, keyword := range keywords {
			if strings.Contains(lowerText, keyword) {
				return crimeType
			}
		}
	}
	return "nao_identificado"
}

// geocodeAddress converte um endereço em coordenadas geográficas usando Nominatim.
func geocodeAddress(address string) (*latlng.LatLng, error) {
	if address == "" {
		return nil, fmt.Errorf("endereço vazio")
	}
	//endpoint de busca da API
	baseURL := "https://nominatim.openstreetmap.org/search"
	fullURL := fmt.Sprintf("%s?format=json&q=%s", baseURL, url.QueryEscape(address))

	req, _ := http.NewRequest("GET", fullURL, nil)
	// User-Agent personalizado e único para cumprir a política de uso da API.
	req.Header.Set("User-Agent", "GeoRisk Scraper Project (joaovitor@gmail.com)")

	client := &http.Client{Timeout: 10 * time.Second}
	//envia a requisição para a API
	res, err := client.Do(req)
	//verifica falhas de conexão
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	//tratamento de erro
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler o corpo da resposta: %v", err)
	}
	res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	//decodifica o JSON retornado pela API
	var results []GeocodeResult
	if err := json.NewDecoder(res.Body).Decode(&results); err != nil {
		log.Printf("Erro ao decodificar JSON. Corpo da resposta recebida: %s", string(bodyBytes))
		return nil, err
	}

	//checa se a API retornou resultados
	if len(results) > 0 {
		//converte as coordenadas de string para float64
		lat, _ := strconv.ParseFloat(results[0].Lat, 64)
		lon, _ := strconv.ParseFloat(results[0].Lon, 64)
		return &latlng.LatLng{Latitude: lat, Longitude: lon}, nil
	}
	//nenhum resultado encontrado
	return nil, fmt.Errorf("nenhum resultado encontrado para o endereço")
}

// extractDataFromArticle extrai o endereço e o tipo de crime do corpo de uma notícia.
func extractDataFromArticle(articleURL string) (string, string) {
	// Faz a requisição HTTP para obter o conteúdo da notícia
	res, err := http.Get(articleURL)
	// Verifica se houve erro na requisição
	if err != nil {
		log.Printf("Erro ao acessar %s: %v", articleURL, err)
		return "", ""
	}
	// Garante que o corpo da resposta será fechado após a leitura
	defer res.Body.Close()
	// Usa a biblioteca goquery para converter o stream de dados HTML em um objeto DOM navegável (doc), permitindo a busca por seletores CSS
	doc, err := goquery.NewDocumentFromReader(res.Body)
	// Verifica se houve erro na leitura do conteúdo
	if err != nil {
		log.Printf("Erro ao ler o conteúdo de %s: %v", articleURL, err)
		return "", ""
	}

	var text string
	// Tenta encontrar conteúdo em blocos comuns de artigo.
	text = doc.Find(".entry-content, .post-text, .materia-conteudo").Text()
	if text == "" {
		text = doc.Find("body").Text()
	}

	// Regex para capturar endereços comuns
	re := regexp.MustCompile(`(?i)((Rua|Avenida|Av\.|Travessa|Praça|Bairro|Jardim|Vila|Rodovia|Parque)\s+[^.,\n]{5,80}(?:\s*(n[oº]?\s*\d+))?)`)

	// Aplica o regex ao texto extraído
	matches := re.FindStringSubmatch(text)

	// Processa o endereço capturado
	var address string
	// Se encontrou um endereço válido
	if len(matches) > 1 {
		// Extrai o endereço bruto
		rawAddress := strings.TrimSpace(matches[1])
		//Remove ruídos que o Nominatim não entenderia, como frases descritivas da notícia.
		//Remove pontuações e excesso de espaços no final.
		cleanedAddress := strings.TrimRight(rawAddress, ",. ")

		//Remove frases comuns de ruído que se seguem ao endereço, ex: "por homem desconhecido", "por volta das"
		noiseRe := regexp.MustCompile(`(?i)(\s+(por|em|de|que|do|da|durante|onde|que|e)\s+.*)`)
		cleanedAddress = noiseRe.ReplaceAllString(cleanedAddress, "")

		//Garante que se a regex capturou algo como "Bairro X", não tenha pontuação no final.
		cleanedAddress = strings.TrimRight(cleanedAddress, ",. ")

		// Constrói a string final para o Nominatim
		address = strings.TrimSpace(cleanedAddress) + ", Limeira, SP, Brasil"
		log.Printf("DEBUG: Endereço para geocodificação: %s", address)
	} else {
		log.Printf("DEBUG: REGEX FAILED. Não foi encontrado endereço específico.")
	}
	// Detecta o tipo de crime no texto da notícia
	crimeType := detectCrimeType(text)
	// Retorna o endereço e o tipo de crime
	return address, crimeType
}

// runScraper contém a lógica principal do processo de scraping.
func runScraper() {
	log.Println("=======================================")
	log.Println("Iniciando nova execução do scraper...")
	// Configuração do Firebase
	ctx := context.Background()

	// Esta linha lê o arquivo "KeyFirebase.json" diretamente da pasta do projeto.
	sa := option.WithCredentialsFile("KeyFirebase.json")
	// =================================================================
	// Inicializa o app do Firebase com as credenciais fornecidas
	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalf("Erro ao inicializar o Firebase. Verifique se o arquivo 'KeyFirebase.json' está na pasta correta. Erro: %v\n", err)
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Erro ao conectar ao Firestore: %v", err)
	}
	// Garante que o cliente do Firestore será fechado ao final da função
	defer client.Close()
	// Lista de URLs para buscar links de notícias
	urlsToScrape := []string{
		"https://www.gazetadelimeira.com.br/noticias/editoria/9",
	}
	// Mapa para armazenar links únicos
	allLinks := make(map[string]bool)
	// Itera sobre as URLs para extrair links
	for _, pageURL := range urlsToScrape {
		// Faz a requisição HTTP para obter o conteúdo da página
		res, err := http.Get(pageURL)
		if err != nil {
			log.Printf("Erro ao buscar links de %s: %v", pageURL, err)
			continue
		}
		// // Usa a biblioteca goquery para converter o stream de dados HTML em um objeto DOM navegável (doc), permitindo a busca por seletores CSS
		doc, err := goquery.NewDocumentFromReader(res.Body)
		if res.Body != nil {
			defer res.Body.Close()
		}
		if err != nil {
			log.Printf("Erro ao ler a página de links %s: %v", pageURL, err)
			continue
		}
		// Extrai todos os links da página
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			// Obtém o atributo href do link
			href, exists := s.Attr("href")
			// Armazena apenas links absolutos
			if exists && strings.HasPrefix(href, "http") {
				allLinks[href] = true
			}
		})
	}

	log.Printf("Total de %d links únicos encontrados para processar.", len(allLinks))
	// Itera sobre todos os links encontrados
	for link := range allLinks {
		// Verifica se o link já foi processado para evitar duplicatas.
		iter := client.Collection("risk_zones").Where("link", "==", link).Limit(1).Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			log.Printf("Erro ao verificar duplicidade para o link %s: %v", link, err)
			continue
		}
		if len(docs) > 0 {
			continue // Pula se o link já existe no banco.
		}

		address, crimeType := extractDataFromArticle(link)

		if address == "" || crimeType == "nao_identificado" {
			continue // Pula se não encontrou informações essenciais.
		}
		//  Realiza a geocodificação do endereço extraído
		coords, err := geocodeAddress(address)
		if err != nil {
			log.Printf("Falha na geocodificação para o endereço '%s': %v", address, err)
			continue
		}

		log.Printf("SUCESSO! Processando e salvando novo link: %s", link)
		// Salva os dados no Firestore
		_, _, err = client.Collection("risk_zones").Add(ctx, map[string]interface{}{
			"latitude":  coords.Latitude,
			"longitude": coords.Longitude,
			"raio":      50,
			"tipo":      crimeType,
			"link":      link,
		})
		if err != nil {
			log.Printf("Falha ao salvar no Firestore: %v", err)
		}

		// Pausa obrigatória para respeitar os limites de uso da API de geocodificação.
		time.Sleep(1 * time.Second)
	}

	log.Println("Scraping concluído.")
	log.Println("=======================================")
}

func main() {
	for {
		log.Println("Iniciando ciclo de scraping...")
		runScraper()

		log.Println("Ciclo finalizado. Dormindo por 24 horas...")
		// Pausa a execução por 24 horas
		time.Sleep(24 * time.Hour)
	}
}
