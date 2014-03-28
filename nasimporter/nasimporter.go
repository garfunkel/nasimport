// nasimporter is a package implementing types and methods used for importing media into a NAS.

package nasimporter

import (
	"regexp"
	"path/filepath"
	"errors"
	"sort"
	"fmt"
	"os"
	"net/http"
	"strings"
	"strconv"
	"github.com/garfunkel/nasimport/constants"
	"github.com/garfunkel/nasimport/mapregexp"
	"github.com/krusty64/tvdb"
	"github.com/StalkR/imdb"
	"github.com/arbovm/levenshtein"
)

const (
	TVRoot = constants.MediaRoot + "TV"
	DocumentaryRoot = constants.MediaRoot + "Documentaries"
	MovieRoot = constants.MediaRoot + "Movies"
)

type NasImporter struct {
	tvShowRegex *mapregexp.MapRegexp
	singleDocumentaryRegex *mapregexp.MapRegexp
	multiDocumentaryRegex *mapregexp.MapRegexp
	seasonDocumentaryRegex *mapregexp.MapRegexp
	yearDocumentaryRegex *mapregexp.MapRegexp
	movieWithYearRegex *mapregexp.MapRegexp
	movieWithoutYearRegex *mapregexp.MapRegexp
	existingTVShowDirs map[string][]string
	existingDocumentaryFiles []string
	existingDocumentaryDirs map[string][]string
	existingMovieFiles []string
	existingMovieDirs map[string][]string
	wordRegex *regexp.Regexp
	tvdbClient *tvdb.TVDB
	imdbClient http.Client
}

type ScoreItem struct {
	value string
	words []string
	score int
}

type ScoreItems []ScoreItem

func (scoreItems ScoreItems) Len() int {
	return len(scoreItems)
}

func (scoreItems ScoreItems) Less(i, j int) bool {
	return scoreItems[i].score < scoreItems[j].score
}

func (scoreItems ScoreItems) Swap(i, j int) {
	scoreItems[i], scoreItems[j] = scoreItems[j], scoreItems[i]
}

func NewNasImporter() NasImporter {
	importer := NasImporter{}

	importer.tvShowRegex = mapregexp.MustCompile(`(?P<name>.+?)[sS]?(?P<series>\d+)[eE]?(?P<episode>\d{2,})(?P<other>.*)\.(?P<ext>[^\.]*)$`)
	importer.singleDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)\.(?P<ext>[^\.]*)$`)
	importer.multiDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)([pP][tT]|part|Part|[eE]|episode|Episode).*?(?P<episode>\d+)\.(?P<ext>[^\.]*)$`)
	importer.yearDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)((year|Year).*)?(?P<year>\d{4}).*?([eE]|episode|Episode|part|Part|pt|PT|Pt).*?(?P<episode>\d+)(?P<other>.*)\.(?P<ext>[^\.]*)$`)
	importer.seasonDocumentaryRegex = mapregexp.MustCompile(`(?P<name>.+?)[sS](?P<series>\d+)[eE](?P<episode>\d+)(?P<other>.*)\.(?P<ext>[^\.]*)$`)

	// I didn't know this but an optional group after a variable length group leads to unexpected results.
	importer.movieWithYearRegex = mapregexp.MustCompile(`(?P<name>.+?)(?P<year>\d{4})(?P<other>.*?)\.(?P<ext>[^\.]*)$`)
	importer.movieWithoutYearRegex = mapregexp.MustCompile(`(?P<name>.+?)\.(?P<ext>[^\.]*)$`)

	importer.wordRegex = regexp.MustCompile("[^\\.\\-_\\+\\s]+")
	importer.tvdbClient = tvdb.Open()

	importer.ReadExistingMedia()

	return importer
}

func (importer *NasImporter) ReadExistingMedia() (err error) {
	_, importer.existingTVShowDirs, err = importer.getFilesDirs(TVRoot)
	importer.existingDocumentaryFiles, importer.existingDocumentaryDirs, err = importer.getFilesDirs(DocumentaryRoot)
	importer.existingMovieFiles, importer.existingMovieDirs, err = importer.getFilesDirs(MovieRoot)

	return
}

func (importer *NasImporter) getFilesDirs(path string) (files []string, dirs map[string][]string, err error) {
	allFiles, err := filepath.Glob(filepath.Join(path, "*"))
	dirs = make(map[string][]string)

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
				dirs[allFile] = importer.wordRegex.FindAllString(filepath.Base(allFile), -1)

			case mode.IsRegular():
				files = append(files, allFile)
		}
	}

	return
}

func (importer *NasImporter) detectTVShow(path string) (order ScoreItems, tvShowFields map[string]string, tvShowTVDBResults *tvdb.GetDetailSeriesData, err error) {
	tvShowFields = importer.tvShowRegex.FindStringSubmatchMap(path)

	if tvShowFields == nil {
		err = errors.New("Not a TV show")

		return
	}

	// If we get here, we may have a new/existing TV show, but it could also still be a doco.
	// Split name of tv show into words, and find the most probable results.
	tvShowWords := importer.wordRegex.FindAllString(tvShowFields["name"], -1)
	order = importer.getDirMatchOrder(importer.existingTVShowDirs, tvShowWords)
	probableTitle := strings.Join(tvShowWords, " ")

	// Search TVDB for results.
	rawSeries, _ := importer.tvdbClient.GetSeries(probableTitle, "")
	tvShowTVDBResults, _ = tvdb.ParseDetailSeriesData(rawSeries)

	return
}

func (importer *NasImporter) detectDocumentary(path string) (order ScoreItems, documentaryFields map[string]string, documentaryTVDBResults *tvdb.GetDetailSeriesData, err error) {
	documentaryRegexes := [...]*mapregexp.MapRegexp{
		importer.seasonDocumentaryRegex,
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

		documentaryWords := importer.wordRegex.FindAllString(documentaryFields["name"], -1)
		order = importer.getDirMatchOrder(importer.existingDocumentaryDirs, documentaryWords)
		fmt.Printf("%#v\n\n\n", order)
		probableTitle := strings.Join(documentaryWords, " ")

		// Search TVDB for results.
		rawSeries, _ := importer.tvdbClient.GetSeries(probableTitle, "")
		documentaryTVDBResults, _ = tvdb.ParseDetailSeriesData(rawSeries)

		//episodeData, _ := importer.tvDB.GetEpisodeBySeasonEp(series.Series[0].Id, 1, 1, "en")
		//episode, _ := tvdb.ParseSingleEpisode(episodeData)

		return
	}

	err = errors.New("Not a documentary")

	return
}

func (importer *NasImporter) detectMovie(path string) (order ScoreItems, movieFields map[string]string, movieIMDBResults []imdb.Title, err error) {
	movieFields = importer.movieWithYearRegex.FindStringSubmatchMap(path)

	if movieFields == nil {
		movieFields = importer.movieWithoutYearRegex.FindStringSubmatchMap(path)
	}

	if movieFields == nil {
		err = errors.New("Not a movie")

		return
	}

	movieWords := importer.wordRegex.FindAllString(movieFields["name"], -1)
	order = importer.getDirMatchOrder(importer.existingMovieDirs, movieWords)
	probableTitle := strings.Join(movieWords, " ")

	// If we have a year, use it to aid our search.z
	year, ok := movieFields["year"]

	if ok {
		probableTitle += " " + year
	}

	// Search IMDB for results.
	movieIMDBResults, err = imdb.SearchTitle(&importer.imdbClient, probableTitle)

	return
}

func (importer *NasImporter) getDirMatchOrder(dirMap map[string][]string, words []string) (order ScoreItems) {
	order = make(ScoreItems, len(dirMap))
	orderIndex := 0

	for dir, dirWords := range dirMap {
		scoreItem := ScoreItem{value: dir, words: dirWords}

		joinedDirWords := strings.Join(dirWords, " ")
		joinedWords := strings.Join(words, " ")

		scoreItem.score = levenshtein.Distance(joinedWords, joinedDirWords)

		/*for _, word := range words {
			for _, dirWord := range dirWords {
				if strings.ToLower(word) == strings.ToLower(dirWord) {
					scoreItem.score++
				}
			}
		}*/

		order[orderIndex] = scoreItem
		orderIndex++
	}

	sort.Sort(order)

	return
}

func (importer *NasImporter) importMKV(path, outPath string) {

}

func (importer *NasImporter) processTVShow(path string, fileFields map[string]string, metadata *tvdb.GetDetailSeriesData) {
	seasonNum, _ := strconv.Atoi(fileFields["series"])
	episodeNum, _ := strconv.Atoi(fileFields["episode"])
	episodeData, _ := importer.tvdbClient.GetEpisodeBySeasonEp(metadata.Series[0].Id, seasonNum, episodeNum, "en")
	episode, _ := tvdb.ParseSingleEpisode(episodeData)
	outPath := fmt.Sprintf("%s/%s/Season %02d/%s S%02dE%02d - %s.mkv", TVRoot, metadata.Series[0].SeriesName, seasonNum, metadata.Series[0].SeriesName, seasonNum, episodeNum, episode.Episode.EpisodeName)

	importer.importMKV(path, outPath)
}

func (importer *NasImporter) processDocumentary(path string, fileFields map[string]string, metadata *tvdb.GetDetailSeriesData) {
	println("here");
}

func (importer *NasImporter) Import(path string) (err error) {
	file := filepath.Base(path)

	fmt.Printf("Importing %s\n", path)
	fmt.Printf("Attempting to detect if this is a TV show...\n")

	tvShowOrder, tvShowFields, tvShowTVDBResults, err := importer.detectTVShow(file)

	fmt.Printf("Attempting to detect if this is a documentary...\n")

	documentaryOrder, documentaryFields, documentaryTVDBResults, err := importer.detectDocumentary(file)

	fmt.Printf("Attempting to detect if this is a movie...\n")

	movieOrder, movieFields, movieIMDBResults, err := importer.detectMovie(file)

	// Logic to decide best type of media.


	println(tvShowOrder, documentaryOrder, movieOrder)
	fmt.Printf("TV FIELDS: %#v\nDOC FIELDS: %#v\nMOVIE FIELDS: %#v\n", tvShowFields, documentaryFields, movieFields)
	println(&tvShowTVDBResults, &documentaryTVDBResults, &movieIMDBResults)

	fmt.Printf("%#v %#v\n", tvShowOrder[0], documentaryOrder[0])

	if len(tvShowOrder) >= len(documentaryOrder) && len(tvShowOrder) >= len(movieOrder) {
		importer.processTVShow(path, tvShowFields, tvShowTVDBResults)
	} else if len(documentaryOrder) >= len(tvShowOrder) && len(documentaryOrder) >= len(movieOrder) {
		importer.processDocumentary(path, documentaryFields, documentaryTVDBResults)
	} else {

	}

	if err != nil {
		fmt.Printf("%s\n", err)
	}

	return
}
