# ☕ Java Spring Boot to SprinGo: The Developer Migration Guide

This Rosetta Stone guide maps Java Spring Boot concepts, annotations, and patterns to their idiomatic Go equivalents in
SprinGo.

---

## 1. Concept & Annotation Mapping

| Java Spring Boot | SprinGo Framework | Concept / Usage |
| :--- | :--- | :--- |
| `@SpringBootApplication` | `framework.Bootstrap(opts).Start()` | Application main entrypoint. |
| `@Component` / `@Service` | `ioc.RegisterBean(name, &Struct{})` | Bean registration in IoC. |
| `@Bean` | `ioc.RegisterFactory(name, fn)` | Factory method returning `T` or `(T, error)`. |
| `@Autowired` | `Field *Type `spring:"beanName"` ` | Field-level dependency injection. |
| `Provider<T>` / `ObjectProvider` | `ioc.Provider[T]` / `*ioc.Provider[T]` | Lazy type-safe bean resolution. |
| `@ConfigurationProperties` | `config.RegisterProperties("prefix", &Cfg{})` | Strongly-typed YAML binding. |
| `@Value("${app.key:default}")` | Struct tag or `config.Get[T]()` | Reading environment variables. |
| `@RestController` / `@RequestMapping`| `web.RegisterRoutes(fn)` + `web.Dispatch` | Chi REST endpoint routing. |
| `@PathVariable` | Struct tag `path:"id"` | Binding URL path variables. |
| `@RequestParam` | Struct tag `query:"page"` | Binding query string parameters. |
| `@RequestBody` | Struct `req DTO` argument in handler | Binding and parsing JSON request body. |
| `@Valid` / JSR-303 | Struct tag `validate:"required,email"` | Declarative DTO validation. |
| `@Transactional` | `database.Transactional(ctx, db, fn)` | Declarative transaction handling. |
| `Propagation.REQUIRES_NEW` | `database.PropagationRequiresNew` | Transaction propagation levels. |
| `@Scheduled(cron = "...")` | `scheduler.Schedule(name, cron, fn)` | Recurring cron task execution. |
| `@ShedLock` | `scheduler.WithLockAtMostFor(...)` | Distributed database cron locking. |
| `@EventListener` | `event.Subscribe("topic", fn)` | Domain event listener / subscriber. |
| `ApplicationEventPublisher` | `event.Publish(topic, payload)` | Event publishing. |
| Spring Boot Actuator | `http://localhost:8080/actuator/*` | Health, beans, metrics, and DLQ console. |
| `SmartLifecycle` | `lifecycle.RegisterInitializer / Shutdown` | Ordered startup and graceful shutdown. |

---

## 2. Code Comparison Examples

### 2.1 Bean Registration & Dependency Injection

#### Java Spring Boot
```java
@Service
public class OrderService {
    @Autowired
    private PaymentGateway paymentGateway;
}
```

#### Go SprinGo
```go
type OrderService struct {
    PaymentGateway PaymentGateway `spring:"paymentGateway"`
}

func init() {
    ioc.RegisterBean("orderService", &OrderService{})
}
```

---

### 2.2 REST Controller & Validation

#### Java Spring Boot
```java
@RestController
@RequestMapping("/api/v1/orders")
public class OrderController {
    @PostMapping
    public ResponseEntity<OrderResponse> createOrder(@Valid @RequestBody CreateOrderDTO dto) {
        OrderResponse res = orderService.create(dto);
        return ResponseEntity.status(HttpStatus.CREATED).body(res);
    }
}
```

#### Go SprinGo
```go
type OrderController struct {
    orderService OrderService `spring:"orderService"`
}

func init() {
    ioc.RegisterBean("orderController", &OrderController{})
    web.RegisterRoutes(func(r chi.Router) {
        c, _ := ioc.Get[OrderController]("orderController")
        r.Post("/orders", web.Dispatch(c.createOrder))
    })
}

func (c *OrderController) createOrder(
    ctx context.Context,
    dto request.CreateOrderDTO,
) (any, error) {
    return c.orderService.Create(ctx, dto)
}
```
