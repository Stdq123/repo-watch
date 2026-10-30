package main

import "fmt"

func (s *Service) ListRepositories() []Repository { return s.monitor.List() }
func (s *Service) NeedsMigration() bool           { return s.monitor.NeedsMigration() }
func (s *Service) ImportLegacy(repos []Repository) error {
	if err := s.monitor.ImportLegacy(repos); err != nil {
		return fmt.Errorf("迁移旧仓库失败：%w", err)
	}
	return nil
}
func (s *Service) AddRepository(r Repository, token string) error {
	if err := s.monitor.Add(r); err != nil {
		return err
	}
	if token != "" {
		if err := s.SaveCredential(r.request(), token); err != nil {
			if rollback := s.monitor.Remove(r.ID); rollback != nil {
				return fmt.Errorf("%v；撤销添加失败：%w", err, rollback)
			}
			return err
		}
	}
	s.monitor.Trigger()
	return nil
}
func (s *Service) RemoveRepository(r Repository) error {
	if err := s.monitor.Remove(r.ID); err != nil {
		return err
	}
	if err := s.ClearCredential(r.request()); err != nil {
		return fmt.Errorf("仓库已移除，但清除令牌失败：%w", err)
	}
	return nil
}
func (s *Service) RefreshNow() { s.monitor.Trigger() }
