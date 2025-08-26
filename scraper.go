package main

import (
	"bytes" // Importado para a melhoria de diagnóstico
	"context"
	"encoding/json"
	"fmt"
	"io" // Importado para a melhoria de diagnóstico
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

// geocodeAddress usa o Nominatim (OpenStreetMap) para obter coordenadas
func geocodeAddress(address string) (*latlng.LatLng, error) {
	if address == "" {
		return nil, fmt.Errorf("endereço vazio")
	}

	baseURL := "https://nominatim.openstreetmap.org/search"
	fullURL := fmt.Sprintf("%s?format=json&q=%s", baseURL, url.QueryEscape(address))

	req, _ := http.NewRequest("GET", fullURL, nil)

	req.Header.Set("User-Agent", "GeoRiskScraper/1.0 (joaovitorevora@gmail.com)")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Erro ao ler o corpo da resposta: %v", err)
		return nil, err
	}

	res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var results []GeocodeResult
	if err := json.NewDecoder(res.Body).Decode(&results); err != nil {
		// Se der erro no JSON, o log abaixo irá imprimir a página HTML que a API retornou
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

// extractDataFromArticle extrai o endereço e tipo de crime do artigo
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

func runScraper() {
	log.Println("=======================================")
	log.Println("Iniciando nova execução do scraper...")

	ctx := context.Background()
	sa := option.WithCredentialsFile("./KeyFirebase.json")
	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalf("Erro ao inicializar o Firebase: %v\n", err)
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
		iter := client.Collection("risk_zones").Where("link", "==", link).Limit(1).Documents(ctx)
		docs, err := iter.GetAll()
		if err != nil {
			log.Printf("Erro ao verificar duplicidade para o link %s: %v", link, err)
			continue
		}

		if len(docs) > 0 {
			continue
		}

		address, crimeType := extractDataFromArticle(link)

		if address == "" || crimeType == "nao_identificado" {
			continue
		}

		coords, err := geocodeAddress(address)
		if err != nil {
			log.Printf("Falha na geocodificação para o endereço '%s' extraído de %s: %v", address, link, err)
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

		time.Sleep(1 * time.Second) // Pausa para não sobrecarregar os serviços
	}

	log.Println("Scraping concluído.")
	log.Println("=======================================")
}

func main() {
	runScraper()
}
