package chatv2

import "sync"

// SessionStore guarda sessões em memória (mapa por customer_id).
// Thread-safe para uso concorrente.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// Get retorna a sessão existente ou cria uma nova.
func (s *SessionStore) Get(customerID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[customerID]
	if !ok {
		sess = &Session{
			CustomerID:     customerID,
			History:        []ChatMessage{},
			OnboardingData: make(map[string]string),
		}
		s.sessions[customerID] = sess
	}
	return sess
}
