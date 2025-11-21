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

	baseURL := "https://nominatim.openstreetmap.org/search"
	fullURL := fmt.Sprintf("%s?format=json&q=%s", baseURL, url.QueryEscape(address))

	req, _ := http.NewRequest("GET", fullURL, nil)
	// User-Agent personalizado e único para cumprir a política de uso da API.
	req.Header.Set("User-Agent", "GeoRisk Scraper Project (seu.email@exemplo.com)") // Coloque um email real aqui

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler o corpo da resposta: %v", err)
	}
	res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var results []GeocodeResult
	if err := json.NewDecoder(res.Body).Decode(&results); err != nil {
		log.Printf("Erro ao decodificar JSON. Corpo da resposta recebida: %s", string(bodyBytes))
		return nil, err
	}

	if len(results) > 0 {
		lat, _ := strconv.ParseFloat(results[0].Lat, 64)
		lon, _ := strconv.ParseFloat(results[0].Lon, 64)
		return &latlng.LatLng{Latitude: lat, Longitude: lon}, nil
	}

	return nil, fmt.Errorf("nenhum resultado encontrado para o endereço")
}

// extractDataFromArticle extrai o endereço e o tipo de crime do corpo de uma notícia.
func extractDataFromArticle(articleURL string) (string, string) {
	res, err := http.Get(articleURL)
	if err != nil {
		log.Printf("Erro ao acessar %s: %v", articleURL, err)
		return "", ""
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Printf("Erro ao ler o conteúdo de %s: %v", articleURL, err)
		return "", ""
	}

	var text string
	text = doc.Find(".entry-content, .post-text, .materia-conteudo").Text()
	if text == "" {
		text = doc.Find("body").Text()
	}

	re := regexp.MustCompile(`(?i)((Rua|Avenida|Av\.|Travessa|Praça|Bairro|Jardim|Vila|Rodovia)\s+[A-ZÁÉÍÓÚÂÊÔÃÕÇ][^.,\n]+)`)
	matches := re.FindStringSubmatch(text)

	var address string
	if len(matches) > 0 {
		address = strings.TrimSpace(matches[0]) + ", Limeira, SP, Brasil"
	}

	crimeType := detectCrimeType(text)
	return address, crimeType
}

// runScraper contém a lógica principal do processo de scraping.
func runScraper() {
	log.Println("=======================================")
	log.Println("Iniciando nova execução do scraper...")

	ctx := context.Background()

	// =================================================================
	// CÓDIGO AJUSTADO PARA RODAR LOCALMENTE
	// Esta linha lê o arquivo "KeyFirebase.json" diretamente da pasta do projeto.
	sa := option.WithCredentialsFile("KeyFirebase.json")
	// =================================================================

	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalf("Erro ao inicializar o Firebase. Verifique se o arquivo 'KeyFirebase.json' está na pasta correta. Erro: %v\n", err)
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Erro ao conectar ao Firestore: %v", err)
	}
	defer client.Close()

	urlsToScrape := []string{
		"https://www.gazetadelimeira.com.br/noticias/editoria/9",
	}
	allLinks := make(map[string]bool)

	for _, pageURL := range urlsToScrape {
		res, err := http.Get(pageURL)
		if err != nil {
			log.Printf("Erro ao buscar links de %s: %v", pageURL, err)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(res.Body)
		if res.Body != nil {
			defer res.Body.Close()
		}
		if err != nil {
			log.Printf("Erro ao ler a página de links %s: %v", pageURL, err)
			continue
		}
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if exists && strings.HasPrefix(href, "http") {
				allLinks[href] = true
			}
		})
	}

	log.Printf("Total de %d links únicos encontrados para processar.", len(allLinks))

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

		coords, err := geocodeAddress(address)
		if err != nil {
			log.Printf("Falha na geocodificação para o endereço '%s': %v", address, err)
			continue
		}

		log.Printf("SUCESSO! Processando e salvando novo link: %s", link)

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

// main é o ponto de entrada do programa.
func main() {
	runScraper()
}
