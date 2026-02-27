param([string]$SK)

$h = @{ apikey = $SK; Authorization = "Bearer $SK" }
$b = "https://iblcjqegepghipkrahcb.supabase.co/rest/v1"
$seeds = "customer_id=not.in.(cust-001,cust-002,cust-003,cust-004,cust-005)"

function Purge($table, $query) {
    try {
        $r = Invoke-WebRequest -Method DELETE "$b/$table`?$query" -Headers $h -UseBasicParsing -ErrorAction Stop
        Write-Host "$($r.StatusCode) OK  $table"
    } catch {
        $body = $_.ErrorDetails.Message
        Write-Host "ERRO    $table  =>  $body"
    }
}

# Sem FK (delete tudo)
Purge "auth_refresh_tokens"       "id=not.is.null"
Purge "auth_password_reset_codes" "id=not.is.null"
Purge "dev_logins"                "cpf=not.is.null"

# Com FK em customer_id (preserva seeds)
Purge "auth_credentials"          $seeds
Purge "pix_keys"                  $seeds
Purge "pix_transfers"             $seeds
Purge "scheduled_transfers"       $seeds
Purge "credit_cards"              $seeds
Purge "bill_payments"             $seeds
Purge "debit_purchases"           $seeds
Purge "favorites"                 $seeds
Purge "notifications"             $seeds
Purge "accounts"                  $seeds

# Por último (referenciada por todas)
Purge "customer_profiles"         $seeds

Write-Host "`nBase limpa. Apenas seeds cust-001 a cust-005 preservados."
