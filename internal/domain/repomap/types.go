package repomap

type Mode string

type RankedItem struct {
	Kind  string `json:"kind"`
	Score int    `json:"score"`
	Path  string `json:"path"`
}

type RepoMap struct {
	Mode  Mode         `json:"mode"`
	Items []RankedItem `json:"items"`
}
