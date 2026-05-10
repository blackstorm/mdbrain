using System.Security.Cryptography;
using System.Text;

namespace Mdbrain.Security;

public interface IPasswordHasher
{
    string Hash(string password);
    bool Verify(string password, string encoded);
}

public sealed class Pbkdf2PasswordHasher : IPasswordHasher
{
    private const int SaltBytes = 16;
    private const int HashBytes = 32;
    private const int Iterations = 210_000;

    public string Hash(string password)
    {
        var salt = RandomNumberGenerator.GetBytes(SaltBytes);
        var hash = Rfc2898DeriveBytes.Pbkdf2(password, salt, Iterations, HashAlgorithmName.SHA256, HashBytes);
        return $"pbkdf2-sha256${Iterations}${Convert.ToHexString(salt).ToLowerInvariant()}${Convert.ToHexString(hash).ToLowerInvariant()}";
    }

    public bool Verify(string password, string encoded)
    {
        try
        {
            return encoded.StartsWith("bcrypt+sha512$", StringComparison.Ordinal)
                ? BuddyBcryptSha512Verifier.Verify(password, encoded)
                : VerifyPbkdf2(password, encoded);
        }
        catch (ArgumentException)
        {
            return false;
        }
        catch (FormatException)
        {
            return false;
        }
        catch (CryptographicException)
        {
            return false;
        }
    }

    private static bool VerifyPbkdf2(string password, string encoded)
    {
        var parts = encoded.Split('$');
        if (parts is not ["pbkdf2-sha256", var iterationsRaw, var saltRaw, var hashRaw]) return false;
        if (!int.TryParse(iterationsRaw, out var iterations))
        {
            return false;
        }

        var salt = Convert.FromHexString(saltRaw);
        var expected = Convert.FromHexString(hashRaw);
        var actual = Rfc2898DeriveBytes.Pbkdf2(password, salt, iterations, HashAlgorithmName.SHA256, expected.Length);
        return CryptographicOperations.FixedTimeEquals(actual, expected);
    }
}

internal static class BuddyBcryptSha512Verifier
{
    private const int SaltBytes = 16;
    private const int RawHashBytes = 24;
    private const int MinCost = 4;
    private const int MaxCost = 31;
    private static readonly BCrypt.Net.BCrypt Bcrypt = new();
    private static readonly CryptRawDelegate CryptRaw = CreateCryptRawDelegate();

    private delegate byte[] CryptRawDelegate(ReadOnlySpan<byte> inputBytes, ReadOnlySpan<byte> saltBytes, int workFactor);

    public static bool Verify(string password, string encoded)
    {
        var parsed = Parse(encoded);
        if (parsed.Hash.Length == RawHashBytes)
        {
            var keyHash = SHA512.HashData(Encoding.UTF8.GetBytes(password));
            var actual = CryptRaw(keyHash, parsed.Salt, parsed.Cost);
            return CryptographicOperations.FixedTimeEquals(actual, parsed.Hash);
        }

        var bcryptHash = Encoding.ASCII.GetString(parsed.Hash);
        var candidate = LegacyCandidateHex(password, parsed.Salt);
        return BCrypt.Net.BCrypt.Verify(candidate, bcryptHash);
    }

    private static BuddyHash Parse(string encoded)
    {
        var parts = encoded.Split('$');
        if (parts is not ["bcrypt+sha512", var saltRaw, var costRaw, var hashRaw])
        {
            throw new FormatException("Malformed bcrypt+sha512 hash");
        }

        var salt = Convert.FromHexString(saltRaw);
        if (salt.Length != SaltBytes)
        {
            throw new FormatException("Malformed bcrypt+sha512 salt");
        }

        if (!int.TryParse(costRaw, out var cost) || cost is < MinCost or > MaxCost)
        {
            throw new FormatException("Invalid bcrypt+sha512 cost");
        }

        var hash = Convert.FromHexString(hashRaw);
        if (hash.Length == 0)
        {
            throw new FormatException("Malformed bcrypt+sha512 hash");
        }

        return new BuddyHash(salt, cost, hash);
    }

    private static string LegacyCandidateHex(string password, byte[] salt)
    {
        var passwordBytes = Encoding.UTF8.GetBytes(password);
        var merged = new byte[passwordBytes.Length + salt.Length];
        Buffer.BlockCopy(passwordBytes, 0, merged, 0, passwordBytes.Length);
        Buffer.BlockCopy(salt, 0, merged, passwordBytes.Length, salt.Length);
        return Convert.ToHexString(SHA512.HashData(merged)).ToLowerInvariant();
    }

    private static CryptRawDelegate CreateCryptRawDelegate()
    {
        var method = typeof(BCrypt.Net.BCrypt).GetMethod(
            "CryptRaw",
            System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance);
        if (method is null)
        {
            throw new InvalidOperationException("BCrypt.Net CryptRaw method not found");
        }

        return method.CreateDelegate<CryptRawDelegate>(Bcrypt);
    }

    private sealed record BuddyHash(byte[] Salt, int Cost, byte[] Hash);
}
