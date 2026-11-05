-- Back to Celsius (°C = (°F − 32) × 5/9), rounded to one decimal.
UPDATE medical_records
SET vitals = (vitals - 'temperatureFahrenheit')
    || jsonb_build_object(
        'temperatureCelsius',
        round((((vitals->>'temperatureFahrenheit')::numeric - 32) * 5 / 9), 1)
    )
WHERE vitals ? 'temperatureFahrenheit';
