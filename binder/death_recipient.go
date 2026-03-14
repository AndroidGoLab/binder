package binder

// DeathRecipient receives notifications when a remote Binder object dies.
type DeathRecipient interface {
	BinderDied()
}
