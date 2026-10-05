package app

import "github.com/inphaseye172/playanything/internal/config"

func saveDefault(a *App) error {
	c := *a.Cfg
	return config.Save(a.Paths, &c)
}
