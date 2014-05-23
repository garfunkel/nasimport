// nasimporter is a package implementing types and methods used for importing media into a NAS.

package nasimporter

import (
	"regexp"
	"path/filepath"
	"errors"
	"sort"
	"fmt"
	"os"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"strconv"
	"os/exec"
	"bytes"
	"runtime"
	"github.com/garfunkel/go-mapregexp"
	"github.com/garfunkel/go-tvdb"
	"github.com/StalkR/imdb"
	"github.com/arbovm/levenshtein"
)

type MediaType int

const (
	TV MediaType = iota
	Documentary
	Movie
)

type MediaSource int

const (
	Unknown MediaSource = iota
	TVTVDB
	DocumentaryTVDB
	MovieIMDB
	MovieLocal
	TVLocal
	DocumentaryLocal
)

type Config struct {
	MediaDirs struct {
		TVDir string `json:"tv"`
		DocumentaryDir string `json:"documentaries"`
		MovieDir string `json:"movies"`
	} `json:"media_dirs"`
	MatroskaMuxers struct {
		MKVMerge string `json:"mkvmerge"`
		FFMPEG string `json:"ffmpeg"`
	} `json:"matroska_muxers"`
	Interface struct {
		NumVisibleResults int `json:"num_visible_results"`
	} `json:"interface"`
}

type NasImporter struct {
	tvShowRegex1 *mapregexp.MapRegexp
	tvShowRegex2 *mapregexp.MapRegexp
	tvShowRegex3 *mapregexp.MapRegexp
	tvShowRegex4 *mapregexp.MapRegexp
	singleDocumentaryRegex *mapregexp.MapRegexp
	multiDocumentaryRegex *mapregexp.MapRegexp
	yearDocumentaryRegex *mapregexp.MapRegexp
	movieWithYearRegex *mapregexp.MapRegexp
	movieWithoutYearRegex *mapregexp.MapRegexp
	existingTVShowDirs []string
	existingDocumentaryFiles []string
	existingDocumentaryDirs []string
	existingMovieFiles []string
	existingMovieDirs []string
	tvdbWebSearchSeriesRegex *regexp.Regexp
	wordRegex *regexp.Regexp
	imdbClient http.Client
	automaticMode bool
	configPath string
	config Config
}

type ScoreItem struct {
	value string
	words []string
	score int
	source MediaSource
	data interface{}
}

type ImportChoice struct {
	mediaType MediaType
	path string
	data *interface{}
}

type ScoreItems []ScoreItem

func (scoreItems ScoreItems) Len() int {
	return len(scoreItems)
}

func (scoreItems ScoreItems) Less(i, j int) bool {
	less := scoreItems[i].score < scoreItems[j].score

	if scoreItems[i].score != scoreItems[j].score {
		return less
	} else {
		return scoreItems[i].source < scoreItems[j].source
	}
}

func (scoreItems ScoreItems) Swap(i, j int) {
	scoreItems[i], scoreItems[j] = scoreItems[j], scoreItems[i]
}

func NewNasImporter(configPath string, automaticMode bool) (importer NasImporter, err error) {
	importer.configPath, err = filepath.Abs(configPath)

	if err != nil {
		return
	}

	importer.ReadConfig()

	// /home/simon/dataFRED/Usenet/Downloads/never/Never.Mind.The.Buzzcocks.UK.S27E05.720p.HDTV.x264-thebeeb.nzb/never.mind.the.buzzcocks.uk.2705.720p.hdtv.x264-thebeeb.mkv
	importer.tvShowRegex1 = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)+([\(\[]?(?P<year>\d{4})[\)\]]?).*?(\.|-|_|\s)+[sS](?P<season>\d+).*?[eE](?P<episode>\d+)(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)
	importer.tvShowRegex2 = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)*[sS](?P<season>\d+).*?[eE](?P<episode>\d+)(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)
	importer.tvShowRegex3 = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)+([\(\[]?(?P<year>\d{4})[\)\]]?).*?(\.|-|_|\s)+(?P<season>\d+)[xX]?(?P<episode>\d{2})(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)
	importer.tvShowRegex4 = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)*(?P<season>\d+)[xX]?(?P<episode>\d{2})(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)
	importer.singleDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)\s*\.(?P<ext>[^\.]*)$`)
	importer.multiDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)*([pP][tT]|part|Part|[eE]|episode|Episode).*?(?P<episode>\d+)\s*\.(?P<ext>[^\.]*)$`)
	importer.yearDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)*((year|Year).*)?(?P<year>(19|[2-9]\d)\d{2}).*?([eE]|episode|Episode|part|Part|pt|PT|Pt).*?(?P<episode>\d+)(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)

	// I didn't know this but an optional group after a variable length group leads to unexpected results.
	importer.movieWithYearRegex = mapregexp.MustCompile(`(?P<name>.+?)(\.|-|_|\s)*(?P<year>(19|[2-9]\d)\d{2})(\.|-|_|\s)*(?P<other>.*?)\s*\.(?P<ext>[^\.]*)$`)
	importer.movieWithoutYearRegex = mapregexp.MustCompile(`(?P<name>.+?)\s*\.(?P<ext>[^\.]*)$`)

	importer.tvdbWebSearchSeriesRegex = regexp.MustCompile(`(?P<before><a href="/\?tab=series&amp;id=)(?P<seriesId>\d+)(?P<after>\&amp;lid=\d*">)`)

	importer.wordRegex = regexp.MustCompile("[^\\.\\-_\\+\\s]+")
	importer.automaticMode = automaticMode

	importer.ReadExistingMedia()

	return
}

func (importer *NasImporter) ReadConfig() (err error) {
	configBytes, err := ioutil.ReadFile(importer.configPath)

	if err != nil {
		return
	}

	err = json.Unmarshal(configBytes, &importer.config)

	return
}

func (importer *NasImporter) ReadExistingMedia() (err error) {
	_, importer.existingTVShowDirs, err = importer.getFilesDirs(importer.config.MediaDirs.TVDir)
	importer.existingDocumentaryFiles, importer.existingDocumentaryDirs, err = importer.getFilesDirs(importer.config.MediaDirs.DocumentaryDir)
	importer.existingMovieFiles, importer.existingMovieDirs, err = importer.getFilesDirs(importer.config.MediaDirs.MovieDir)

	return
}

func (importer *NasImporter) getFilesDirs(path string) (files []string, dirs []string, err error) {
	allFiles, err := filepath.Glob(filepath.Join(path, "*"))

	if err != nil {
		return
	}

	for _, allFile := range allFiles {
		handle, err := os.Open(allFile)

		if err != nil {
			continue
		}

		stat, err := handle.Stat()

		if err != nil {
			continue
		}

		switch mode := stat.Mode(); {
			case mode.IsDir():
				dirs = append(dirs, filepath.Base(allFile))

			case mode.IsRegular():
				files = append(files, filepath.Base(allFile))
		}
	}

	return
}

func (importer *NasImporter) detectTVShowFields(path string) (tvShowFields map[string]string, err error) {
	tvRegexes := [...]*mapregexp.MapRegexp{
		importer.tvShowRegex1,
		importer.tvShowRegex2,
		importer.tvShowRegex3,
		importer.tvShowRegex4,
	}

	// Try the different tvRegexes.
	for _, tvRegex := range tvRegexes {
		tvShowFields = tvRegex.FindStringSubmatchMap(path)

		if tvShowFields == nil {
			continue
		}

		return
	}

	err = errors.New("Not a TV show")

	return
}

func (importer *NasImporter) detectTVShow(name string) (order ScoreItems, err error) {
	// If we get here, we may have a new/existing TV show, but it could also still be a doco.
	// Split name of tv show into words, and find the most probable results.
	order = importer.getLevenshteinOrder(importer.existingTVShowDirs, name)

	return
}

func (importer *NasImporter) detectTvdbSeries(name, genre string) (seriesList tvdb.SeriesList, err error) {
	words := importer.wordRegex.FindAllString(name, -1)
	probableTitle := strings.Join(words, " ")

	// Search TVDB for results.
	seriesList, err = tvdb.SearchSeries(probableTitle, importer.config.Interface.NumVisibleResults)

	if genre != "" {
		genreSeriesList := tvdb.SeriesList{}
		negate := false

		if genre[0] == '!' {
			negate = true
			genre = genre[1 :]
		}

		for _, series := range seriesList.Series {
			for _, seriesGenre := range series.Genre {
				if (strings.ToLower(genre) == strings.ToLower(seriesGenre)) != negate {
					genreSeriesList.Series = append(genreSeriesList.Series, series)

					break
				}
			}
		}

		seriesList = genreSeriesList
	}

	return
}

func (importer *NasImporter) detectDocumentaryFields(path string) (documentaryFields map[string]string, err error) {
	documentaryRegexes := [...]*mapregexp.MapRegexp{
		importer.tvShowRegex1,
		importer.tvShowRegex2,
		importer.tvShowRegex3,
		importer.tvShowRegex4,
		importer.yearDocumentaryRegex,
		importer.multiDocumentaryRegex,
		importer.singleDocumentaryRegex,
	}

	// Try the different documentaryRegexes in order of complexity as a singleDocumentaryRegex will almost always return results.
	for _, documentaryRegex := range documentaryRegexes {
		documentaryFields = documentaryRegex.FindStringSubmatchMap(path)

		if documentaryFields == nil {
			continue
		}

		return
	}

	err = errors.New("Not a documentary")

	return
}

func (importer *NasImporter) detectDocumentary(name string) (order ScoreItems, err error) {
	order = importer.getLevenshteinOrder(importer.existingDocumentaryDirs, name)

	return
}

func (importer *NasImporter) detectMovieFields(path string) (movieFields map[string]string, err error) {
	movieFields = importer.movieWithYearRegex.FindStringSubmatchMap(path)

	if movieFields == nil {
		movieFields = importer.movieWithoutYearRegex.FindStringSubmatchMap(path)
	}

	if movieFields == nil {
		err = errors.New("Not a movie")
	}

	return
}

func (importer *NasImporter) detectMovie(name string) (order ScoreItems, err error) {
	order = importer.getLevenshteinOrder(importer.existingMovieDirs, name)

	return
}

func (importer *NasImporter) detectIMDBMovie(name string) (movieIMDBResults []imdb.Title, err error) {
	movieWords := importer.wordRegex.FindAllString(name, -1)
	probableTitle := strings.Join(movieWords, " ")

	// Search IMDB for results.
	// Ignore error, it seems that if no results are found we get an error.
	rawMovieIMDBResults, _ := imdb.SearchTitle(&importer.imdbClient, probableTitle)
	idMap := make(map[string]struct{})

	for _, rawMovieIMDBResult := range rawMovieIMDBResults {
		if _, ok := idMap[rawMovieIMDBResult.ID]; !ok {
			idMap[rawMovieIMDBResult.ID] = struct{}{}

			movieIMDBResults = append(movieIMDBResults, rawMovieIMDBResult)
		}
	}

	return
}

func (importer *NasImporter) getLevenshteinDistance(string1, string2 string) int {
	string1Words := importer.wordRegex.FindAllString(string1, -1)
	joinedString1Words := strings.Join(string1Words, " ")
	string2Words := importer.wordRegex.FindAllString(string2, -1)
	joinedString2Words := strings.Join(string2Words, " ")

	return levenshtein.Distance(strings.ToLower(joinedString1Words), strings.ToLower(joinedString2Words))
}

func (importer *NasImporter) getLevenshteinOrder(candidates []string, target string) (order ScoreItems) {
	targetWords := importer.wordRegex.FindAllString(target, -1)
	joinedTargetWords := strings.Join(targetWords, " ")
	order = make(ScoreItems, len(candidates))
	orderIndex := 0

	for _, candidate := range candidates {
		candidateWords := importer.wordRegex.FindAllString(candidate, -1)
		scoreItem := ScoreItem{value: candidate, words: candidateWords}
		joinedCandidateWords := strings.Join(candidateWords, " ")
		scoreItem.score = levenshtein.Distance(strings.ToLower(joinedTargetWords), strings.ToLower(joinedCandidateWords))
		order[orderIndex] = scoreItem
		orderIndex++
	}

	sort.Sort(order)

	return
}

func (importer *NasImporter) importMKVUsingMKVMerge(path, outPath string) (err error) {
	cmd := exec.Command(importer.config.MatroskaMuxers.MKVMerge, "-o", outPath, path)
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		err = errors.New(importer.config.MatroskaMuxers.MKVMerge + " exited with error: " + strings.TrimSpace(stdout.String()))
	}

	return
}

func (importer *NasImporter) importMKVUsingFFMPEG(path, outPath string) (err error) {
	cmd := exec.Command(importer.config.MatroskaMuxers.FFMPEG, "-fflags", "+genpts", "-i", path, "-codec", "copy", "-y", outPath)
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		err = errors.New(importer.config.MatroskaMuxers.FFMPEG + " exited with error: " + strings.TrimSpace(stderr.String()))
	}

	return
}

func (importer *NasImporter) importMKV(path, outPath string) (err error) {
	if _, err = os.Stat(outPath); !os.IsNotExist(err) {
		return
	}

	err = os.MkdirAll(filepath.Dir(outPath), os.ModeDir | 0755)

	if err != nil {
		return
	}

	err = importer.importMKVUsingMKVMerge(path, outPath)

	if err == nil {
		return 
	}

	fmt.Println(err)

	err = importer.importMKVUsingFFMPEG(path, outPath)

	if err == nil {
		return
	}

	fmt.Println(err)

	return
}

func (importer *NasImporter) importTV(path string, fileFields map[string]string, data interface{}) (err error) {
	seasonNum, err := strconv.ParseUint(fileFields["season"], 10, 64)

	if err != nil {
		return
	}

	episodeNum, err := strconv.ParseUint(fileFields["episode"], 10, 64)

	if err != nil {
		return
	}

	seriesName := ""
	episodeName := ""
	outPath := ""

	switch data.(type) {
		case tvdb.Series:
			castData := data.(tvdb.Series)
			seriesName = castData.SeriesName

			if castData.Seasons == nil {
				if err = castData.GetDetail(); err != nil {
					return
				}
			}

			season, ok := castData.Seasons[seasonNum]

			if !ok {
				err = errors.New(fmt.Sprintf("Season %v doesn't exist on TheTVDB.", seasonNum))

				return
			} else {
				for _, episode := range season {
					if episode.EpisodeNumber == episodeNum {
						episodeName = episode.EpisodeName

						break
					}
				}
			}

			if episodeName == "" {
				err = errors.New(fmt.Sprintf("Episode %v doesn't exist on TheTVDB.", episodeNum))

				return
			}

			// Replace ASCII slash with unicode division slash in file name parts.
			seriesName = strings.Replace(seriesName, "/", "∕", -1)
			episodeName = strings.Replace(episodeName, "/", "∕", -1)

			if runtime.GOOS == "windows" {
				m := regexp.MustCompile(`[\\\?\%\*\:\|\"\<\>\=\,\;\[\]]`)

				seriesName = m.ReplaceAllString(seriesName, "")
				episodeName = m.ReplaceAllString(episodeName, "")
			}

			outPath = fmt.Sprintf("%s/%s/Season %02d/%s S%02dE%02d - %s.mkv", importer.config.MediaDirs.TVDir, seriesName, seasonNum, seriesName, seasonNum, episodeNum, episodeName)
		case string:
			// Replace ASCII slash with unicode division slash in file name parts.
			seriesName = strings.Replace(data.(string), "/", "∕", -1)

			if runtime.GOOS == "windows" {
				m := regexp.MustCompile(`[\\\?\%\*\:\|\"\<\>\=\,\;\[\]]`)

				seriesName = m.ReplaceAllString(seriesName, "")
			}

			outPath = fmt.Sprintf("%s/%s/Season %02d/%s S%02dE%02d.mkv", importer.config.MediaDirs.TVDir, seriesName, seasonNum, seriesName, seasonNum, episodeNum)
	}

	err = importer.importMKV(path, outPath)

	return
}

func (importer *NasImporter) importDocumentary(path string, fileFields map[string]string, data interface{}) (err error) {
	return
}

func (importer *NasImporter) importMovie(path string, fileFields map[string]string, data interface{}) (err error) {
	movie := data.(imdb.Title)
	outPath := fmt.Sprintf("%v/%v (%v).mkv", importer.config.MediaDirs.MovieDir, movie.Name, movie.Year)
	err = importer.importMKV(path, outPath)

	return
}

func (importer *NasImporter) Import(path string) (err error) {
	path, err = filepath.Abs(path)

	if err != nil {
		return
	}

	fileInfo, err := os.Stat(path)

	if os.IsNotExist(err) {
		return
	}

	if fileInfo.IsDir() {
		err = errors.New(fmt.Sprintf("%v is a directory.", path))

		return
	}

	file := filepath.Base(path)

	fmt.Printf("Importing %s\n", path)
	fmt.Printf("Attempting to detect if this is a TV show...\n")

	tvShowFields, err := importer.detectTVShowFields(file)
	tvShowOrder := ScoreItems{}
	tvShowTVDBResults := tvdb.SeriesList{}

	if err == nil {
		tvShowOrder, err = importer.detectTVShow(tvShowFields["name"])

		if err != nil {
			log.Fatal(err)
		}

		tvShowTVDBResults, err = importer.detectTvdbSeries(tvShowFields["name"], "")

		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println(err)
	}

	fmt.Printf("Attempting to detect if this is a documentary...\n")

	documentaryFields, err := importer.detectDocumentaryFields(file)
	documentaryOrder := ScoreItems{}
	documentaryTVDBResults := tvdb.SeriesList{}

	if err == nil {
		documentaryOrder, err = importer.detectDocumentary(documentaryFields["name"])

		if err != nil {
			log.Fatal(err)
		}

		documentaryTVDBResults, err = importer.detectTvdbSeries(documentaryFields["name"], "documentary")

		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println(err)
	}

	fmt.Printf("Attempting to detect if this is a movie...\n")

	movieFields, err := importer.detectMovieFields(file)
	movieOrder := ScoreItems{}
	movieIMDBResults := []imdb.Title{}

	if err == nil {
		movieOrder, err = importer.detectMovie(movieFields["name"])

		if err != nil {
			log.Fatal(err)
		}

		movieName := movieFields["name"]

		// If we have a year, use it to aid our search.z
		movieYear, ok := movieFields["year"]

		if ok {
			movieName += " (" + movieYear + ")"
		}

		movieIMDBResults, err = importer.detectIMDBMovie(movieName)

		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println(err)
	}

	// Logic to decide best type of media.
	fmt.Printf("TV FIELDS: %#v\nDOC FIELDS: %#v\nMOVIE FIELDS: %#v\n", tvShowFields, documentaryFields, movieFields)

	fmt.Printf("Most likely TV show matches (local):\n")

	//choices := make(map[int]ImportChoice)
	absoluteOrder := ScoreItems{}

	for index, tvShow := range tvShowOrder {
		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v\n", tvShow.value)
		}

		tvShow.source = TVLocal
		tvShow.data = tvShow.value
		absoluteOrder = append(absoluteOrder, tvShow)
	}

	fmt.Printf("\nMost likely TV show matches (TheTVDB):\n")

	for index, tvShowTVDBResult := range tvShowTVDBResults.Series {
		score := 0

		if year, ok := tvShowFields["year"]; ok {
			score = importer.getLevenshteinDistance(fmt.Sprintf(tvShowFields["name"] + " (%v)", year), fmt.Sprintf(tvShowTVDBResult.SeriesName))
		} else {
			score = importer.getLevenshteinDistance(tvShowFields["name"], tvShowTVDBResult.SeriesName)
		}

		scoreItem := ScoreItem{value: tvShowTVDBResult.SeriesName, score: score, source: TVTVDB, data: tvShowTVDBResult}
		absoluteOrder = append(absoluteOrder, scoreItem)

		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v\n", tvShowTVDBResult.SeriesName)
		}
	}

	fmt.Printf("\nMost likely documentary matches (local):\n")

	for index, documentary := range documentaryOrder {
		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v\n", documentary.value)
		}

		documentary.source = DocumentaryLocal
		documentary.data = documentary.value
		absoluteOrder = append(absoluteOrder, documentary)
	}

	fmt.Printf("\nMost likely documentary matches (TheTVDB):\n")

	for index, documentaryTVDBResult := range documentaryTVDBResults.Series {
		score := importer.getLevenshteinDistance(documentaryFields["name"], documentaryTVDBResult.SeriesName)
		scoreItem := ScoreItem{value: documentaryTVDBResult.SeriesName, score: score, source: DocumentaryTVDB, data: documentaryTVDBResult}
		absoluteOrder = append(absoluteOrder, scoreItem)

		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v\n", documentaryTVDBResult.SeriesName)
		}
	}

	fmt.Printf("\nMost likely movie matches (local):\n")

	for index, movie := range movieOrder {
		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v\n", movie.value)
		}

		movie.source = MovieLocal
		movie.data = movie.value
		absoluteOrder = append(absoluteOrder, movie)
	}

	fmt.Printf("\nMost likely movie matches (IMDb):\n")

	for index, movieIMDBResult := range movieIMDBResults {
		score := 0

		if year, ok := movieFields["year"]; ok {
			score = importer.getLevenshteinDistance(fmt.Sprintf(movieFields["name"] + " (%v)", year), fmt.Sprintf(movieIMDBResult.Name + " (%v)", movieIMDBResult.Year))
		} else {
			score = importer.getLevenshteinDistance(movieFields["name"], movieIMDBResult.Name)
		}

		scoreItem := ScoreItem{value: movieIMDBResult.Name, score: score, source: MovieIMDB, data: movieIMDBResult}
		absoluteOrder = append(absoluteOrder, scoreItem)

		if index < importer.config.Interface.NumVisibleResults {
			fmt.Printf("\t%v (%v)\n", movieIMDBResult.Name, movieIMDBResult.Year)
		}
	}

	sort.Sort(absoluteOrder)

	fmt.Printf("\nMost likely overall matches:\n")

	for index, result := range absoluteOrder {
		if index < importer.config.Interface.NumVisibleResults {
			source := "unknown"

			switch result.source {
				case TVLocal:
					source = "local TV show"
				case DocumentaryLocal:
					source = "local documentary"
				case MovieLocal:
					source = "local movie"
				case TVTVDB:
					series := result.data.(tvdb.Series)
					source = fmt.Sprintf("TheTVDB TV show (ID: %v)", series.Id)
				case DocumentaryTVDB:
					series := result.data.(tvdb.Series)
					source = fmt.Sprintf("TheTVDB documentary (ID: %v)", series.Id)
				case MovieIMDB:
					movie := result.data.(imdb.Title)
					source = fmt.Sprintf("IMDb movie (ID: %v)", movie.ID)
			}

			if result.source == MovieIMDB {
				movie := result.data.(imdb.Title)

				fmt.Printf("%v \t%v | %v (%v)\n\t\t%v\n", result.score, index + 1, result.value, movie.Year, source)
			} else {
				fmt.Printf("%v \t%v | %v\n\t\t%v\n", result.score, index + 1, result.value, source)
			}
		} else {
			break
		}
	}

	matchId := 1

	if importer.automaticMode {
		fmt.Println("\nAutomatically choosing ID: 1")
	} else {
		for {
			fmt.Printf("\nEnter ID: ")

			_, err := fmt.Scanf("%d", &matchId)

			if err != nil || matchId > importer.config.Interface.NumVisibleResults || matchId < 1 {
				fmt.Printf("\nSorry, invalid ID. Try again.\n")
			} else {
				break
			}
		}
	}

	match := absoluteOrder[matchId - 1]

	switch match.source {
		case TVLocal:
			err = importer.importTV(path, tvShowFields, match.data)

		case TVTVDB:
			err = importer.importTV(path, tvShowFields, match.data)

		case DocumentaryLocal:
			err = importer.importDocumentary(path, documentaryFields, match.data)

		case DocumentaryTVDB:
			err = importer.importDocumentary(path, documentaryFields, match.data)

		case MovieLocal:
			err = importer.importMovie(path, movieFields, match.data)

		case MovieIMDB:
			err = importer.importMovie(path, movieFields, match.data)
	}

	return
}
