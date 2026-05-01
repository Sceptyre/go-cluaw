package memory

import (
	"path/filepath"
)

type Get struct {
	store *Store
	index *Index
}

func NewGet(workspace string) *Get {
	return &Get{
		store: NewStore(workspace),
		index: NewIndex(workspace),
	}
}

func (g *Get) Today() (string, error) {
	return g.store.GetTodayContent()
}

func (g *Get) ByReference(ref string) (string, error) {
	return g.index.GetReference(ref)
}

func (g *Get) ByTag(tag string) (string, error) {
	return g.index.SearchByTag(tag)
}

func (g *Get) ByDate(date string) (string, error) {
	return g.store.GetFile(date + ".md")
}

func (g *Get) Recent(count int) ([]string, error) {
	return g.index.GetRecentFiles(count)
}

func (g *Get) Workspace() string {
	return g.store.workspace
}

func (g *Get) Path() string {
	return filepath.Join(g.store.workspace, "memory")
}
