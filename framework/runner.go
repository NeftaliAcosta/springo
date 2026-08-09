package framework

import (
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"log"
	"sort"
)

// CommandLineRunner allows executing custom logic after the application starts.
type CommandLineRunner interface {
	Run(args []string) error
}

// Ordered allows defining execution priorities for components.
// Lower values run first (e.g., -10 runs before 0, which runs before 10).
type Ordered interface {
	GetOrder() int
}

// RunCommandLineRunners finds all beans in the container implementing CommandLineRunner,
// sorts them according to the Ordered interface (or bean name fallback), and executes them.
func (a *Application) RunCommandLineRunners(args []string) {
	container := ioc.GetContainer()
	beans := container.GetAllBeans()

	var runners []CommandLineRunner
	runnerNames := make(map[CommandLineRunner]string)

	for name, bean := range beans {
		if r, ok := bean.(CommandLineRunner); ok {
			runners = append(runners, r)
			runnerNames[r] = name
		}
	}

	if len(runners) == 0 {
		return
	}

	// Sort runners by GetOrder() or alphabetically by bean name if orders are equal
	sort.Slice(runners, func(i, j int) bool {
		orderI := 0
		orderJ := 0

		if ord, ok := runners[i].(Ordered); ok {
			orderI = ord.GetOrder()
		}
		if ord, ok := runners[j].(Ordered); ok {
			orderJ = ord.GetOrder()
		}

		if orderI == orderJ {
			return runnerNames[runners[i]] < runnerNames[runners[j]]
		}

		return orderI < orderJ
	})

	log.Printf("🚀 [CommandLineRunner] Found %d runner(s) in container. Starting execution...", len(runners))

	for _, runner := range runners {
		name := runnerNames[runner]
		log.Printf("⏳ [CommandLineRunner] Executing: %s", name)
		if err := runner.Run(args); err != nil {
			log.Printf("❌ [CommandLineRunner] Error executing runner %s: %v", name, err)
		} else {
			log.Printf("✅ [CommandLineRunner] Runner %s executed successfully", name)
		}
	}
}
