// nasimporter is a package implementing types and methods used for importing media into a NAS.

package nasimporter

import (
	"NAS/nasimport/constants"
	"fmt"
	"os"
	"NAS/nasimport/mapregexp"
	"regexp"
	"path/filepath"
	"errors"
	"sort"
)

const (
	TVRoot = constants.MediaRoot + "TV"
	DocumentaryRoot = constants.MediaRoot + "Documentaries"
	MovieRoot = constants.MediaRoot + "Movies"
)

type NasImporter struct {
	tvShowRegex mapregexp.MapRegexp
	existingTVShowDirs map[string][]string
	existingDocumentaryFiles []string
	existingDocumentaryDirs map[string][]string
	existingMovieFiles []string
	existingMovieDirs map[string][]string
	wordRegex regexp.Regexp
}

type ScoreItem struct {
	value string
	words []string
	score uint
}

type ScoreItems []ScoreItem

func (scoreItems ScoreItems) Len() int {
	return len(scoreItems)
}

func (scoreItems ScoreItems) Less(i, j int) bool {
	return scoreItems[i].score > scoreItems[j].score
}

func (scoreItems ScoreItems) Swap(i, j int) {
	scoreItems[i], scoreItems[j] = scoreItems[j], scoreItems[i]
}

func NewNasImporter() NasImporter {
	importer := NasImporter{}

	importer.tvShowRegex = *mapregexp.MustCompile("(?P<name>.+?)[sS](?P<series>\\d+)[eE](?P<episode>\\d+)(?P<other>.*)\\.(?P<ext>.*?)$")
	importer.wordRegex = *regexp.MustCompile("[^\\.\\-_\\+\\s]+")

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

func (importer *NasImporter) detectTVShow(path string) (order ScoreItems, err error) {
	tvShowFields := importer.tvShowRegex.FindStringSubmatchMap(path)

	if tvShowFields == nil {
		err = errors.New("Not a TV show")

		return
	}

	// If we get here, we may have a new/existing TV show, but it could also still be a doco.

	// Split name of tv show into words, and find the most probable results.
	tvShowWords := importer.wordRegex.FindAllString(tvShowFields["name"], -1)
	order = importer.getDirMatchOrder(importer.existingTVShowDirs, tvShowWords)

	return
}

func (importer *NasImporter) detectDocumentary(path string) (order ScoreItems, err error) {
	return
}

func (importer *NasImporter) detectMovie(path string) (order ScoreItems, err error) {
	return
}

func (importer *NasImporter) getDirMatchOrder(dirMap map[string][]string, words []string) (order ScoreItems) {
	order = make(ScoreItems, len(dirMap))
	orderIndex := 0

	for dir, dirWords := range dirMap {
		scoreItem := ScoreItem{value: dir, words: dirWords}

		for _, word := range words {
			for _, dirWord := range dirWords {
				if word == dirWord {
					scoreItem.score++
				}
			}
		}

		order[orderIndex] = scoreItem
		orderIndex++
	}

	sort.Sort(order)

	return
}

func (importer *NasImporter) Import(path string) (err error) {
	fmt.Printf("Importing %s\n", path)
	fmt.Printf("Attempting to detect if this is a TV show...\n")

	file := filepath.Base(path)
	tvShowOrder, err := importer.detectTVShow(file)
	documentaryOrder, err := importer.detectDocumentary(file)
	movieOrder, err := importer.detectMovie(file)

	println(tvShowOrder, documentaryOrder, movieOrder)

	if err != nil {
		fmt.Printf("%s\n", err)
	}

	return
}
