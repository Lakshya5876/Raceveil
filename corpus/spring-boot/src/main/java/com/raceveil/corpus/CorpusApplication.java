package com.raceveil.corpus;

import java.security.SecureRandom;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import javax.sql.DataSource;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

/**
 * RaceVeil Phase 2 seeded corpus — Spring Boot + PostgreSQL.
 *
 * A subset of three race archetypes (Design/TEST_PLAN.md Layer 5),
 * reimplemented in a second stack so detection isn't overfit to one
 * framework's response idioms: coupon redemption (max_successes),
 * inventory overrun (monotonic_limit, SELECT ... FOR UPDATE), and order
 * confirmation (single_transition). Each has a vulnerable (check-then-act,
 * two separate statements, no lock) and synchronized-safe twin. Local-only:
 * binds 127.0.0.1, talks to the local-only Postgres container in
 * corpus/docker-compose.yml. Nothing here is deployed or billed anywhere.
 */
@SpringBootApplication
@RestController
public class CorpusApplication {

    private static final SecureRandom RNG = new SecureRandom();
    private static final long RACE_WINDOW_MS = 8; // widens the check-then-act window, same rationale as the Node/Go fixtures

    private final JdbcTemplate jdbc;

    @Autowired
    public CorpusApplication(DataSource dataSource) {
        this.jdbc = new JdbcTemplate(dataSource);
    }

    public static void main(String[] args) {
        SpringApplication.run(CorpusApplication.class, args);
    }

    private static String randomId() {
        byte[] b = new byte[8];
        RNG.nextBytes(b);
        StringBuilder sb = new StringBuilder();
        for (byte x : b) {
            sb.append(String.format("%02x", x));
        }
        return sb.toString();
    }

    private static void sleepRaceWindow() {
        try {
            Thread.sleep(RACE_WINDOW_MS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }

    // -----------------------------------------------------------------
    // Archetype: check-then-act coupon redemption (max_successes).
    // -----------------------------------------------------------------

    @PostMapping("/coupon/issue")
    public Map<String, Object> issueCoupon() {
        String code = randomId();
        jdbc.update("INSERT INTO coupons_vuln_spring (code, used) VALUES (?, false)", code);
        return Map.of("code", code);
    }

    @PostMapping("/coupon/redeem")
    public ResponseEntity<Map<String, Object>> redeemCoupon(@RequestBody Map<String, String> body) {
        String code = body.get("code");
        List<Boolean> rows = jdbc.query("SELECT used FROM coupons_vuln_spring WHERE code = ?",
                (rs, i) -> rs.getBoolean("used"), code);
        if (rows.isEmpty() || rows.get(0)) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "already used"));
        }
        sleepRaceWindow();
        jdbc.update("UPDATE coupons_vuln_spring SET used = true WHERE code = ?", code);
        return ResponseEntity.ok(redeemedBody());
    }

    @PostMapping("/coupon/issue-safe")
    public Map<String, Object> issueCouponSafe() {
        String code = randomId();
        jdbc.update("INSERT INTO coupons_safe_spring (code, used) VALUES (?, false)", code);
        return Map.of("code", code);
    }

    @PostMapping("/coupon/redeem-safe")
    public ResponseEntity<Map<String, Object>> redeemCouponSafe(@RequestBody Map<String, String> body) {
        String code = body.get("code");
        int updated = jdbc.update("UPDATE coupons_safe_spring SET used = true WHERE code = ? AND used = false", code);
        if (updated == 0) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "already used"));
        }
        return ResponseEntity.ok(redeemedBody());
    }

    private static Map<String, Object> redeemedBody() {
        Map<String, Object> m = new HashMap<>();
        m.put("status", "redeemed");
        m.put("receipt_id", randomId());
        return m;
    }

    // -----------------------------------------------------------------
    // Archetype: inventory overrun on the last unit (monotonic_limit).
    // vuln: SELECT stock then UPDATE (two statements, no lock). safe: an
    // explicit transaction with SELECT ... FOR UPDATE, the textbook
    // Postgres row-locking pattern.
    // -----------------------------------------------------------------

    @PostMapping("/inventory/restock")
    public Map<String, Object> restock() {
        String sku = "sku_" + randomId();
        jdbc.update("INSERT INTO inventory_vuln_spring (sku, stock) VALUES (?, 1)", sku);
        return Map.of("sku", sku);
    }

    @PostMapping("/inventory/reserve")
    public ResponseEntity<Map<String, Object>> reserve(@RequestBody Map<String, String> body) {
        String sku = body.get("sku");
        List<Integer> rows = jdbc.query("SELECT stock FROM inventory_vuln_spring WHERE sku = ?",
                (rs, i) -> rs.getInt("stock"), sku);
        if (rows.isEmpty() || rows.get(0) <= 0) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "out of stock"));
        }
        sleepRaceWindow();
        jdbc.update("UPDATE inventory_vuln_spring SET stock = stock - 1 WHERE sku = ?", sku);
        return ResponseEntity.ok(redeemedBody());
    }

    @PostMapping("/inventory/restock-safe")
    public Map<String, Object> restockSafe() {
        String sku = "sku_" + randomId();
        jdbc.update("INSERT INTO inventory_safe_spring (sku, stock) VALUES (?, 1)", sku);
        return Map.of("sku", sku);
    }

    @PostMapping("/inventory/reserve-safe")
    @Transactional
    public ResponseEntity<Map<String, Object>> reserveSafe(@RequestBody Map<String, String> body) {
        String sku = body.get("sku");
        List<Integer> rows = jdbc.query("SELECT stock FROM inventory_safe_spring WHERE sku = ? FOR UPDATE",
                (rs, i) -> rs.getInt("stock"), sku);
        if (rows.isEmpty() || rows.get(0) <= 0) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "out of stock"));
        }
        jdbc.update("UPDATE inventory_safe_spring SET stock = stock - 1 WHERE sku = ?", sku);
        return ResponseEntity.ok(redeemedBody());
    }

    // -----------------------------------------------------------------
    // Archetype: duplicate "confirm" state transition (single_transition).
    // -----------------------------------------------------------------

    @PostMapping("/order/create")
    public Map<String, Object> createOrder() {
        String id = "order_" + randomId();
        jdbc.update("INSERT INTO orders_vuln_spring (id, status) VALUES (?, 'pending')", id);
        return Map.of("id", id);
    }

    @PostMapping("/order/confirm")
    public ResponseEntity<Map<String, Object>> confirmOrder(@RequestBody Map<String, String> body) {
        String id = body.get("id");
        List<String> rows = jdbc.query("SELECT status FROM orders_vuln_spring WHERE id = ?",
                (rs, i) -> rs.getString("status"), id);
        if (rows.isEmpty() || !"pending".equals(rows.get(0))) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "not pending"));
        }
        sleepRaceWindow();
        jdbc.update("UPDATE orders_vuln_spring SET status = 'confirmed' WHERE id = ?", id);
        return ResponseEntity.ok(confirmedBody());
    }

    @PostMapping("/order/create-safe")
    public Map<String, Object> createOrderSafe() {
        String id = "order_" + randomId();
        jdbc.update("INSERT INTO orders_safe_spring (id, status) VALUES (?, 'pending')", id);
        return Map.of("id", id);
    }

    @PostMapping("/order/confirm-safe")
    public ResponseEntity<Map<String, Object>> confirmOrderSafe(@RequestBody Map<String, String> body) {
        String id = body.get("id");
        int updated = jdbc.update(
                "UPDATE orders_safe_spring SET status = 'confirmed' WHERE id = ? AND status = 'pending'", id);
        if (updated == 0) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "not pending"));
        }
        return ResponseEntity.ok(confirmedBody());
    }

    private static Map<String, Object> confirmedBody() {
        Map<String, Object> m = new HashMap<>();
        m.put("status", "confirmed");
        m.put("receipt_id", randomId());
        return m;
    }
}
