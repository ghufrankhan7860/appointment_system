-- Vitals temperature is now recorded in Fahrenheit: move vitals.temperatureCelsius to
-- vitals.temperatureFahrenheit (°F = °C × 9/5 + 32), rounded to one decimal.
UPDATE medical_records
SET vitals = (vitals - 'temperatureCelsius')
    || jsonb_build_object(
        'temperatureFahrenheit',
        round(((vitals->>'temperatureCelsius')::numeric * 9 / 5 + 32), 1)
    )
WHERE vitals ? 'temperatureCelsius';
