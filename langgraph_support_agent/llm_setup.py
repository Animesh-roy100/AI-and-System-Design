import os
from dotenv import load_dotenv
from langchain_openai import ChatOpenAI

# Load environment variables, particularly OpenAI and LangSmith keys
load_dotenv()

def get_llm(temperature: float = 0.0) -> ChatOpenAI:
    """
    Initializes and returns the ChatOpenAI model.
    By default, sets temperature to 0 for deterministic, reliable routing and reasoning.
    """
    return ChatOpenAI(
        model="gpt-4o-mini",  # Using the faster, cost-effective model suitable for agents
        temperature=temperature
    )
